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
  listK8sClusterRefs,
  getK8sClusterRef,
  createK8sClusterRef,
  updateK8sClusterRef,
  deleteK8sClusterRef,
} from '../../api/strategy'
import type { K8sClusterRef, CreateK8sClusterRefPayload } from '../../types/strategy'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const loading = ref(false)
const submitting = ref(false)
const dataSource = ref<K8sClusterRef[]>([])
const drawerVisible = ref(false)
const drawerTitle = ref('新建 K8s 目标实例')
const formRef = ref<FormInstance>()
const editingID = ref('')

const formState = reactive<{
  code: string
  cluster_name: string
  environment_code: string
  api_server: string
  default_namespace: string
  argocd_instance_id: string
  supports_native_strategy: boolean
  supports_rollouts: boolean
  traffic_provider: string
}>({
  code: '',
  cluster_name: '',
  environment_code: '',
  api_server: '',
  default_namespace: '',
  argocd_instance_id: '',
  supports_native_strategy: false,
  supports_rollouts: false,
  traffic_provider: '',
})

const accessModeLabels: Record<string, string> = {
  in_cluster: '集群内',
  external: '外部',
}

const columns: TableColumnsType = [
  { title: '编码', dataIndex: 'code', key: 'code', width: 120 },
  { title: '集群名称', dataIndex: 'cluster_name', key: 'cluster_name', ellipsis: true },
  { title: '环境', dataIndex: 'environment_code', key: 'environment_code', width: 100 },
  { title: 'API Server', dataIndex: 'api_server', key: 'api_server', ellipsis: true },
  { title: '默认命名空间', dataIndex: 'default_namespace', key: 'default_namespace', width: 130 },
  { title: '访问模式', dataIndex: 'access_mode', key: 'access_mode', width: 90 },
  { title: '原生策略', dataIndex: 'supports_native_strategy', key: 'supports_native_strategy', width: 90 },
  { title: 'Rollouts', dataIndex: 'supports_rollouts', key: 'supports_rollouts', width: 90 },
  {
    title: '操作',
    key: 'action',
    width: 140,
    fixed: 'right',
  },
]

async function loadList() {
  loading.value = true
  try {
    const res = await listK8sClusterRefs()
    dataSource.value = res.data.items ?? []
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '加载 K8s 目标实例失败'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingID.value = ''
  drawerTitle.value = '新建 K8s 目标实例'
  formState.code = ''
  formState.cluster_name = ''
  formState.environment_code = ''
  formState.api_server = ''
  formState.default_namespace = ''
  formState.argocd_instance_id = ''
  formState.supports_native_strategy = false
  formState.supports_rollouts = false
  formState.traffic_provider = ''
  formRef.value?.clearValidate()
  drawerVisible.value = true
}

async function openEdit(id: string) {
  try {
    const res = await getK8sClusterRef(id)
    const item = res.data
    editingID.value = id
    drawerTitle.value = '编辑 K8s 目标实例'
    formState.code = item.code
    formState.cluster_name = item.cluster_name
    formState.environment_code = item.environment_code
    formState.api_server = item.api_server
    formState.default_namespace = item.default_namespace
    formState.argocd_instance_id = item.argocd_instance_id
    formState.supports_native_strategy = item.supports_native_strategy
    formState.supports_rollouts = item.supports_rollouts
    formState.traffic_provider = item.traffic_provider
    formRef.value?.clearValidate()
    drawerVisible.value = true
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '加载 K8s 目标实例详情失败'))
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
    const payload: CreateK8sClusterRefPayload = {
      code: formState.code,
      cluster_name: formState.cluster_name,
      environment_code: formState.environment_code,
      api_server: formState.api_server,
      default_namespace: formState.default_namespace,
      argocd_instance_id: formState.argocd_instance_id,
      supports_native_strategy: formState.supports_native_strategy,
      supports_rollouts: formState.supports_rollouts,
      traffic_provider: formState.traffic_provider,
    }
    if (editingID.value) {
      await updateK8sClusterRef(editingID.value, payload)
      message.success('K8s 目标实例已更新')
    } else {
      await createK8sClusterRef(payload)
      message.success('K8s 目标实例创建成功')
    }
    drawerVisible.value = false
    await loadList()
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '保存 K8s 目标实例失败'))
  } finally {
    submitting.value = false
  }
}

async function handleDelete(id: string) {
  Modal.confirm({
    title: '确认删除',
    content: '删除后不可恢复，确定要删除该 K8s 目标实例吗？',
    okText: '确认删除',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await deleteK8sClusterRef(id)
        message.success('K8s 目标实例已删除')
        await loadList()
      } catch (err) {
        message.error(extractHTTPErrorMessage(err, '删除 K8s 目标实例失败'))
      }
    },
  })
}

onMounted(() => {
  loadList()
})
</script>

<template>
  <div class="k8s-cluster-ref-page">
    <div class="page-header">
      <div class="page-header-title">
        <h3 class="page-title">K8s 目标实例</h3>
        <span class="page-desc">管理 K8s 集群连接信息，供应用环境绑定运行时</span>
      </div>
      <div class="page-header-actions">
        <a-button type="default" @click="loadList" :loading="loading">
          <ReloadOutlined />
        </a-button>
        <a-button type="primary" @click="openCreate">
          <PlusOutlined />
          新建实例
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
        :scroll="{ x: 1100 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'access_mode'">
            <a-tag>{{ accessModeLabels[record.access_mode] || record.access_mode }}</a-tag>
          </template>
          <template v-else-if="column.key === 'supports_native_strategy'">
            <a-tag :color="record.supports_native_strategy ? 'green' : 'default'">
              {{ record.supports_native_strategy ? '是' : '否' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'supports_rollouts'">
            <a-tag :color="record.supports_rollouts ? 'green' : 'default'">
              {{ record.supports_rollouts ? '是' : '否' }}
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
        :label-col="{ span: 6 }"
        :wrapper-col="{ span: 18 }"
        layout="horizontal"
      >
        <a-form-item
          label="编码"
          name="code"
          :rules="[{ required: true, message: '请输入集群编码' }]"
        >
          <a-input v-model:value="formState.code" placeholder="例如：prod-k8s-01" />
        </a-form-item>
        <a-form-item
          label="集群名称"
          name="cluster_name"
          :rules="[{ required: true, message: '请输入集群名称' }]"
        >
          <a-input v-model:value="formState.cluster_name" placeholder="例如：生产集群" />
        </a-form-item>
        <a-form-item
          label="环境编码"
          name="environment_code"
          :rules="[{ required: true, message: '请输入环境编码' }]"
        >
          <a-input v-model:value="formState.environment_code" placeholder="例如：production" />
        </a-form-item>
        <a-form-item
          label="API Server"
          name="api_server"
          :rules="[{ required: true, message: '请输入 API Server 地址' }]"
        >
          <a-input v-model:value="formState.api_server" placeholder="例如：https://k8s.example.com:6443" />
        </a-form-item>
        <a-form-item label="默认命名空间" name="default_namespace">
          <a-input v-model:value="formState.default_namespace" placeholder="例如：default" />
        </a-form-item>
        <a-form-item label="ArgoCD 实例ID" name="argocd_instance_id">
          <a-input v-model:value="formState.argocd_instance_id" placeholder="关联的 ArgoCD 实例" />
        </a-form-item>
        <a-form-item label="原生策略支持" name="supports_native_strategy">
          <a-switch v-model:checked="formState.supports_native_strategy" />
        </a-form-item>
        <a-form-item label="Argo Rollouts" name="supports_rollouts">
          <a-switch v-model:checked="formState.supports_rollouts" />
        </a-form-item>
        <a-form-item label="流量提供商" name="traffic_provider">
          <a-input v-model:value="formState.traffic_provider" placeholder="例如：istio、nginx、none" />
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
.k8s-cluster-ref-page {
  padding: 0;
}
</style>
