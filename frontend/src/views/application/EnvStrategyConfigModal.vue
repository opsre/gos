<script setup lang="ts">
import { message } from 'ant-design-vue'
import { computed, onMounted, reactive, ref } from 'vue'
import {
  listK8sClusterRefs,
  listRuntimeBindings,
  createRuntimeBinding,
  updateRuntimeBinding,
  listStrategyTemplates,
  listStrategyBindings,
  createStrategyBinding,
  updateStrategyBinding,
} from '../../api/strategy'
import type { K8sClusterRef, StrategyTemplate, AppEnvRuntimeBinding, AppEnvStrategyBinding } from '../../types/strategy'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const props = defineProps<{
  applicationId: string
  envCode: string
}>()

const emit = defineEmits<{
  saved: []
}>()

const visible = ref(false)
const runtimeLoading = ref(false)
const strategyLoading = ref(false)
const runtimeSubmitting = ref(false)
const strategySubmitting = ref(false)

const clusterRefs = ref<K8sClusterRef[]>([])
const templates = ref<StrategyTemplate[]>([])

const existingRuntimeBinding = ref<AppEnvRuntimeBinding | null>(null)
const existingStrategyBinding = ref<AppEnvStrategyBinding | null>(null)

const runtimeForm = reactive({
  k8s_cluster_ref_id: '',
  namespace: '',
  workload_name: '',
})

const strategyForm = reactive({
  strategy_template_id: '',
  overrides_config: '',
})

const clusterRefOptions = computed(() =>
  clusterRefs.value.map((c) => ({
    label: `${c.cluster_name} (${c.code})`,
    value: c.id,
  })),
)

const templateOptions = computed(() =>
  templates.value.map((t) => ({
    label: `${t.name} (${t.strategy_engine} · ${t.strategy_type})`,
    value: t.id,
  })),
)

async function loadClusterRefs() {
  try {
    const res = await listK8sClusterRefs()
    clusterRefs.value = res.data.items ?? []
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '加载 K8s 目标实例失败'))
  }
}

async function loadTemplates() {
  try {
    const res = await listStrategyTemplates({ status: 'active' })
    templates.value = res.data.items ?? []
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '加载策略模板失败'))
  }
}

async function loadRuntimeBinding() {
  if (!props.applicationId || !props.envCode) return
  runtimeLoading.value = true
  try {
    const res = await listRuntimeBindings({
      application_id: props.applicationId,
      env_code: props.envCode,
    })
    const items = res.data.items ?? []
    if (items.length > 0) {
      existingRuntimeBinding.value = items[0]
      runtimeForm.k8s_cluster_ref_id = items[0].k8s_cluster_ref_id
      runtimeForm.namespace = items[0].namespace
      runtimeForm.workload_name = items[0].workload_name
    } else {
      existingRuntimeBinding.value = null
      runtimeForm.k8s_cluster_ref_id = ''
      runtimeForm.namespace = ''
      runtimeForm.workload_name = ''
    }
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '加载运行时绑定失败'))
  } finally {
    runtimeLoading.value = false
  }
}

async function loadStrategyBinding() {
  if (!props.applicationId || !props.envCode) return
  strategyLoading.value = true
  try {
    const res = await listStrategyBindings({
      application_id: props.applicationId,
      env_code: props.envCode,
    })
    const items = res.data.items ?? []
    if (items.length > 0) {
      existingStrategyBinding.value = items[0]
      strategyForm.strategy_template_id = items[0].strategy_template_id
      strategyForm.overrides_config = items[0].overrides_config
    } else {
      existingStrategyBinding.value = null
      strategyForm.strategy_template_id = ''
      strategyForm.overrides_config = ''
    }
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '加载策略绑定失败'))
  } finally {
    strategyLoading.value = false
  }
}

async function handleRuntimeSave() {
  if (!runtimeForm.k8s_cluster_ref_id) {
    message.warning('请选择 K8s 目标实例')
    return
  }
  if (!runtimeForm.namespace) {
    message.warning('请输入命名空间')
    return
  }
  runtimeSubmitting.value = true
  try {
    if (existingRuntimeBinding.value) {
      await updateRuntimeBinding(existingRuntimeBinding.value.id, {
        k8s_cluster_ref_id: runtimeForm.k8s_cluster_ref_id,
        namespace: runtimeForm.namespace,
        workload_name: runtimeForm.workload_name,
      })
      message.success('运行时配置已更新')
    } else {
      await createRuntimeBinding({
        application_id: props.applicationId,
        env_code: props.envCode,
        k8s_cluster_ref_id: runtimeForm.k8s_cluster_ref_id,
        namespace: runtimeForm.namespace,
        workload_name: runtimeForm.workload_name,
      })
      message.success('运行时配置已创建')
    }
    await loadRuntimeBinding()
    emit('saved')
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '保存运行时配置失败'))
  } finally {
    runtimeSubmitting.value = false
  }
}

async function handleStrategySave() {
  if (!strategyForm.strategy_template_id) {
    message.warning('请选择策略模板')
    return
  }
  strategySubmitting.value = true
  try {
    if (existingStrategyBinding.value) {
      await updateStrategyBinding(existingStrategyBinding.value.id, {
        strategy_template_id: strategyForm.strategy_template_id,
        overrides_config: strategyForm.overrides_config,
      })
      message.success('策略配置已更新')
    } else {
      await createStrategyBinding({
        application_id: props.applicationId,
        env_code: props.envCode,
        strategy_template_id: strategyForm.strategy_template_id,
        overrides_config: strategyForm.overrides_config,
      })
      message.success('策略配置已创建')
    }
    await loadStrategyBinding()
    emit('saved')
  } catch (err) {
    message.error(extractHTTPErrorMessage(err, '保存策略配置失败'))
  } finally {
    strategySubmitting.value = false
  }
}

function open() {
  visible.value = true
  loadRuntimeBinding()
  loadStrategyBinding()
}

function close() {
  visible.value = false
}

onMounted(() => {
  loadClusterRefs()
  loadTemplates()
})

defineExpose({ open, close })
</script>

<template>
  <a-modal
    :open="visible"
    title="环境策略与运行时配置"
    width="640"
    :footer="null"
    @cancel="close"
  >
    <a-spin :spinning="runtimeLoading || strategyLoading">
      <a-card title="K8s 运行时" :bordered="true" size="small" style="margin-bottom: 16px">
        <a-form
          :label-col="{ span: 6 }"
          :wrapper-col="{ span: 18 }"
          layout="horizontal"
          size="small"
        >
          <a-form-item label="K8s 目标实例">
            <a-select
              v-model:value="runtimeForm.k8s_cluster_ref_id"
              :options="clusterRefOptions"
              placeholder="选择 K8s 集群"
              show-search
              option-filter-prop="label"
            />
          </a-form-item>
          <a-form-item label="命名空间">
            <a-input v-model:value="runtimeForm.namespace" placeholder="例如：default" />
          </a-form-item>
          <a-form-item label="工作负载名称">
            <a-input v-model:value="runtimeForm.workload_name" placeholder="例如：my-app" />
          </a-form-item>
          <a-form-item :wrapper-col="{ offset: 6, span: 18 }">
            <a-button type="primary" size="small" :loading="runtimeSubmitting" @click="handleRuntimeSave">
              {{ existingRuntimeBinding ? '更新运行时' : '创建运行时' }}
            </a-button>
          </a-form-item>
        </a-form>
      </a-card>

      <a-card title="发布策略" :bordered="true" size="small">
        <a-form
          :label-col="{ span: 6 }"
          :wrapper-col="{ span: 18 }"
          layout="horizontal"
          size="small"
        >
          <a-form-item label="策略模板">
            <a-select
              v-model:value="strategyForm.strategy_template_id"
              :options="templateOptions"
              placeholder="选择策略模板"
              show-search
              option-filter-prop="label"
            />
          </a-form-item>
          <a-form-item label="覆盖配置">
            <a-textarea
              v-model:value="strategyForm.overrides_config"
              :rows="4"
              placeholder='JSON 格式的自定义配置，留空使用模板默认值'
            />
          </a-form-item>
          <a-form-item :wrapper-col="{ offset: 6, span: 18 }">
            <a-button type="primary" size="small" :loading="strategySubmitting" @click="handleStrategySave">
              {{ existingStrategyBinding ? '更新策略' : '创建策略' }}
            </a-button>
          </a-form-item>
        </a-form>
      </a-card>
    </a-spin>
  </a-modal>
</template>
