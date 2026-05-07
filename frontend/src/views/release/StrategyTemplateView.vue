<script setup lang="ts">
import {
  PlusOutlined,
  ReloadOutlined,
  DeleteOutlined,
  EditOutlined,
} from '@ant-design/icons-vue'
import { message, Modal } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import { onMounted, reactive, ref } from 'vue'
import {
  listStrategyTemplates,
  getStrategyTemplate,
  createStrategyTemplate,
  updateStrategyTemplate,
  deleteStrategyTemplate,
} from '../../api/strategy'
import type {
  StrategyTemplate,
  CreateStrategyTemplatePayload,
} from '../../types/strategy'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const loading = ref(false)
const submitting = ref(false)
const dataSource = ref<StrategyTemplate[]>([])
const drawerVisible = ref(false)
const drawerTitle = ref('新建策略模板')
const formRef = ref<FormInstance>()
const editingID = ref('')

const formState = reactive<{
  name: string
  strategy_engine: string
  strategy_type: string
  strategy_config: string
  description: string
  status: string
}>({
  name: '',
  strategy_engine: 'argocd',
  strategy_type: 'rolling_update',
  strategy_config: '',
  description: '',
  status: 'active',
})

const engineOptions = [
  { label: 'ArgoCD', value: 'argocd' },
  { label: 'GitOps', value: 'gitops' },
]
const typeOptions = [
  { label: '滚动更新 (RollingUpdate)', value: 'rolling_update' },
  { label: '蓝绿发布 (BlueGreen)', value: 'blue_green' },
  { label: '金丝雀发布 (Canary)', value: 'canary' },
]
const statusOptions = [
  { label: '启用', value: 'active' },
  { label: '禁用', value: 'inactive' },
]

const columns: TableColumnsType = [
  { title: '模板名称', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '策略引擎', dataIndex: 'strategy_engine', key: 'strategy_engine', width: 110 },
  { title: '策略类型', dataIndex: 'strategy_type', key: 'strategy_type', width: 130 },
  { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
  { title: '状态', dataIndex: 'status', key: 'status', width: 80 },
  {
    title: '操作',
    key: 'action',
    width: 140,
    fixed: 'right',
  },
]

const engineLabel = (v: string) => engineOptions.find((o) => o.value === v)?.label ?? v
const typeLabel = (v: string) => typeOptions.find((o) => o.value === v)?.label ?? v
const statusLabel = (v: string) => statusOptions.find((o) => o.value === v)?.label ?? v

async function loadList() {
  loading.value = true
  try {
    const res = await listStrategyTemplates()
    dataSource.value = res.data.items ?? []
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '加载策略模板失败'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingID.value = ''
  drawerTitle.value = '新建策略模板'
  formState.name = ''
  formState.strategy_engine = 'argocd'
  formState.strategy_type = 'rolling_update'
  formState.strategy_config = ''
  formState.description = ''
  formState.status = 'active'
  formRef.value?.clearValidate()
  drawerVisible.value = true
}

async function openEdit(id: string) {
  try {
    const res = await getStrategyTemplate(id)
    const tpl = res.data
    editingID.value = id
    drawerTitle.value = '编辑策略模板'
    formState.name = tpl.name
    formState.strategy_engine = tpl.strategy_engine
    formState.strategy_type = tpl.strategy_type
    formState.strategy_config = tpl.strategy_config
    formState.description = tpl.description
    formState.status = tpl.status
    formRef.value?.clearValidate()
    drawerVisible.value = true
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '加载策略模板详情失败'))
  }
}

async function handleSubmit() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  submitting.value = true
  try {
    const payload: CreateStrategyTemplatePayload = {
      name: formState.name,
      strategy_engine: formState.strategy_engine,
      strategy_type: formState.strategy_type,
      strategy_config: formState.strategy_config,
      description: formState.description,
    }
    if (editingID.value) {
      await updateStrategyTemplate(editingID.value, {
        ...payload,
        status: formState.status,
      })
      message.success('策略模板已更新')
    } else {
      await createStrategyTemplate(payload)
      message.success('策略模板创建成功')
    }
    drawerVisible.value = false
    await loadList()
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '保存策略模板失败'))
  } finally {
    submitting.value = false
  }
}

async function handleDelete(id: string) {
  Modal.confirm({
    title: '确认删除',
    content: '删除后不可恢复，确定要删除该策略模板吗？',
    okText: '确认删除',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await deleteStrategyTemplate(id)
        message.success('策略模板已删除')
        await loadList()
      } catch (err) {
        message.error(extractHTTPErrorMessage(err, '删除策略模板失败'))
      }
    },
  })
}

onMounted(() => {
  loadList()
})
</script>

<template>
  <div class="strategy-template-page">
    <div class="page-header">
      <div class="page-header-title">
        <h3 class="page-title">策略模板</h3>
        <span class="page-desc">管理 K8s 发布策略模板，定义发布策略的默认配置</span>
      </div>
      <div class="page-header-actions">
        <a-button type="default" @click="loadList" :loading="loading">
          <ReloadOutlined />
        </a-button>
        <a-button type="primary" @click="openCreate">
          <PlusOutlined />
          新建模板
        </a-button>
      </div>
    </div>

    <a-card :bordered="true" class="table-card">
      <a-table
        :columns="columns"
        :data-source="dataSource"
        :loading="loading"
        row-key="id"
        :pagination="false"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'strategy_engine'">
            {{ engineLabel(record.strategy_engine) }}
          </template>
          <template v-else-if="column.key === 'strategy_type'">
            {{ typeLabel(record.strategy_type) }}
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="record.status === 'active' ? 'green' : 'default'">
              {{ statusLabel(record.status) }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-button size="small" type="link" @click="openEdit(record.id)">
                <EditOutlined />
              </a-button>
              <a-button size="small" type="link" danger @click="handleDelete(record.id)">
                <DeleteOutlined />
              </a-button>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer
      :open="drawerVisible"
      :title="drawerTitle"
      width="560"
      @close="drawerVisible = false"
    >
      <a-form
        ref="formRef"
        :model="formState"
        :label-col="{ span: 5 }"
        :wrapper-col="{ span: 19 }"
        layout="horizontal"
      >
        <a-form-item
          label="模板名称"
          name="name"
          :rules="[{ required: true, message: '请输入模板名称' }]"
        >
          <a-input v-model:value="formState.name" placeholder="例如：标准滚动更新" />
        </a-form-item>
        <a-form-item
          label="策略引擎"
          name="strategy_engine"
          :rules="[{ required: true, message: '请选择策略引擎' }]"
        >
          <a-select v-model:value="formState.strategy_engine" :options="engineOptions" />
        </a-form-item>
        <a-form-item
          label="策略类型"
          name="strategy_type"
          :rules="[{ required: true, message: '请选择策略类型' }]"
        >
          <a-select v-model:value="formState.strategy_type" :options="typeOptions" />
        </a-form-item>
        <a-form-item label="策略配置" name="strategy_config">
          <a-textarea
            v-model:value="formState.strategy_config"
            :rows="8"
            placeholder='JSON 格式的策略默认配置，例如 {"maxSurge":"25%","maxUnavailable":"25%"}'
          />
        </a-form-item>
        <a-form-item label="描述" name="description">
          <a-textarea
            v-model:value="formState.description"
            :rows="2"
            placeholder="简要描述模板的用途"
          />
        </a-form-item>
        <a-form-item v-if="editingID" label="状态" name="status">
          <a-select v-model:value="formState.status" :options="statusOptions" />
        </a-form-item>
      </a-form>
      <template #footer>
        <a-space style="float: right">
          <a-button @click="drawerVisible = false">取消</a-button>
          <a-button type="primary" :loading="submitting" @click="handleSubmit">
            {{ editingID ? '保存' : '创建' }}
          </a-button>
        </a-space>
      </template>
    </a-drawer>
  </div>
</template>

<style scoped>
.strategy-template-page {
  padding: 0;
}
</style>
