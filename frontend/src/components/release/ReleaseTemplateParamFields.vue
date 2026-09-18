<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  getExecutorParamDefByID,
  listApplicationExecutorParamDefs,
} from '../../api/pipeline'
import { getReleaseTemplateByID } from '../../api/release'
import type { ExecutorParamDef } from '../../types/pipeline'
import type { ReleaseAutomationParam } from '../../types/release-automation'
import type { ReleasePipelineScope, ReleaseTemplateParam } from '../../types/release'
import { resolveChoiceMeta, type SelectOption } from '../../utils/executor-param-choice'

/**
 * 模板映射出来的参数填写区：只渲染「发布时填写（release_input）」的参数，
 * 固定值 / 基础字段 / 继承 CI 的参数由模板自己处理，不在这里出现。
 */
interface FieldRow {
  scope: ReleasePipelineScope
  param: ReleaseTemplateParam
  def: ExecutorParamDef | null
  label: string
  required: boolean
  options: SelectOption[]
  multiple: boolean
  delimiter: string
}

const props = withDefaults(
  defineProps<{
    applicationId: string
    templateId: string
    /** 本次派发涉及的管线：仅构建=ci、构建并发布=ci+cd、仅发布=cd。 */
    scopes: ReleasePipelineScope[]
    /** 编辑已有配置时用落库的参数回填（按 scope + 参数键匹配）。 */
    initialParams?: ReleaseAutomationParam[]
    disabled?: boolean
  }>(),
  {
    initialParams: () => [],
    disabled: false,
  },
)

// 参数值由本组件维护，变化后直接把「建单可用的参数数组」和校验结果抛给父组件
const emit = defineEmits<{
  (e: 'update:params', value: ReleaseAutomationParam[]): void
  (e: 'update:error', value: string): void
  /** 模板把 CI 分支写死时的取值；父级据此回填并锁定「分支」输入框。 */
  (e: 'update:fixedGitRef', value: string): void
}>()

const loading = ref(false)
const templateParams = ref<ReleaseTemplateParam[]>([])
const paramDefs = ref<Record<string, ExecutorParamDef>>({})
const values = reactive<Record<string, string>>({})
const submitError = ref('')

const activeScopes = computed(() => new Set(props.scopes.map((scope) => scope.trim().toLowerCase())))

function isReleaseInputParam(param: ReleaseTemplateParam) {
  const source = String(param.value_source || '').trim().toLowerCase()
  return source === '' || source === 'release_input'
}

function resolveFieldLabel(param: ReleaseTemplateParam, def: ExecutorParamDef | null) {
  return (
    String(param.param_name || '').trim() ||
    String(def?.param_key || '').trim() ||
    String(param.param_key || '').trim() ||
    String(param.executor_param_name || '').trim() ||
    String(def?.executor_param_name || '').trim() ||
    '参数'
  )
}

/** 只需要填的：模板开放为发布输入，且落在本次派发的管线里。 */
const visibleParams = computed(() =>
  templateParams.value
    .filter((param) => isReleaseInputParam(param))
    .filter((param) => activeScopes.value.has(String(param.pipeline_scope || '').trim().toLowerCase()))
    .slice()
    .sort((left, right) => (left.sort_no || 0) - (right.sort_no || 0)),
)

const fields = computed<FieldRow[]>(() =>
  visibleParams.value.map((param) => {
    const def = paramDefs.value[param.executor_param_def_id] || null
    const meta = def ? resolveChoiceMeta(def) : null
    return {
      scope: param.pipeline_scope,
      param,
      def,
      label: resolveFieldLabel(param, def),
      required: Boolean(param.required) || isReleaseInputParam(param),
      options: meta?.options || [],
      multiple: Boolean(meta?.multiple),
      delimiter: meta?.delimiter || ',',
    }
  }),
)

const visibleFields = computed(() => fields.value)

/** 模板把 CI 的 GIT_REF 固定住时，发布分支就由模板决定，页面上不必再让用户填。 */
const fixedGitRef = computed(() => {
  const matched = templateParams.value.find((param) => {
    const scope = String(param.pipeline_scope || '').trim().toLowerCase()
    const key = String(param.param_key || '').trim().toLowerCase()
    const source = String(param.value_source || '').trim().toLowerCase()
    return scope === 'ci' && key === 'git_ref' && source === 'fixed'
  })
  return String(matched?.fixed_value || '').trim()
})

async function loadParamDefs(bindingID: string, scope: ReleasePipelineScope, param: ReleaseTemplateParam) {
  if (!props.applicationId || !bindingID) {
    return null
  }
  try {
    const response = await listApplicationExecutorParamDefs(props.applicationId, {
      binding_type: scope,
      binding_id: bindingID,
      status: 'active',
      page: 1,
      page_size: 200,
    })
    const matched = response.data.find((item) => item.id === param.executor_param_def_id)
    if (matched) {
      return matched
    }
  } catch {
    // 应用级列表拿不到时退回单条查询，避免参数区一直空着
  }
  try {
    const detail = await getExecutorParamDefByID(param.executor_param_def_id)
    return detail.data
  } catch {
    return null
  }
}

function resetValues() {
  Object.keys(values).forEach((key) => {
    delete values[key]
  })
  fields.value.forEach((field) => {
    values[field.param.executor_param_def_id] = ''
  })
}

/** 编辑态：把已落库的参数值按管线 + 参数键回填到对应控件。 */
function applyInitialParams() {
  if (props.initialParams.length === 0) {
    return
  }
  const index = new Map<string, string>()
  props.initialParams.forEach((item) => {
    const scope = String(item.pipeline_scope || '').trim().toLowerCase()
    const key = String(item.param_key || '').trim().toLowerCase()
    const executorName = String(item.executor_param_name || '').trim().toLowerCase()
    index.set(`${scope}|${key}|${executorName}`, String(item.param_value || ''))
    index.set(`${scope}|${key}|`, String(item.param_value || ''))
  })
  fields.value.forEach((field) => {
    const scope = String(field.param.pipeline_scope || '').trim().toLowerCase()
    const key = String(field.param.param_key || '').trim().toLowerCase()
    const executorName = String(field.def?.executor_param_name || field.param.executor_param_name || '')
      .trim()
      .toLowerCase()
    const next =
      index.get(`${scope}|${key}|${executorName}`) ??
      index.get(`${scope}|${key}|`) ??
      index.get(`${scope}||${executorName}`) ??
      ''
    if (next) {
      values[field.param.executor_param_def_id] = next
    }
  })
}

async function loadTemplateFields() {
  const templateID = String(props.templateId || '').trim()
  if (!templateID || !props.applicationId) {
    templateParams.value = []
    paramDefs.value = {}
    resetValues()
    return
  }
  loading.value = true
  try {
    const response = await getReleaseTemplateByID(templateID)
    templateParams.value = response.data.params || []
    const needed = visibleParams.value
    const pairs = await Promise.all(
      needed.map(async (param) => {
        const def = await loadParamDefs(param.binding_id, param.pipeline_scope, param)
        return [param.executor_param_def_id, def] as const
      }),
    )
    const nextDefs: Record<string, ExecutorParamDef> = {}
    pairs.forEach(([id, def]) => {
      if (def) {
        nextDefs[id] = def
      }
    })
    paramDefs.value = nextDefs
    resetValues()
    applyInitialParams()
  } catch {
    templateParams.value = []
    paramDefs.value = {}
    resetValues()
  } finally {
    loading.value = false
  }
}

function fieldValue(field: FieldRow) {
  return values[field.param.executor_param_def_id] || ''
}

function setFieldValue(field: FieldRow, value: string) {
  values[field.param.executor_param_def_id] = value
}

function setMultiValue(field: FieldRow, next: string[]) {
  setFieldValue(field, next.filter(Boolean).join(field.delimiter))
}

function multiValue(field: FieldRow) {
  const raw = fieldValue(field)
  if (!raw) {
    return [] as string[]
  }
  return raw
    .split(field.delimiter)
    .map((item) => item.trim())
    .filter(Boolean)
}

function choiceSelectValue(field: FieldRow) {
  if (field.multiple) {
    return multiValue(field)
  }
  return fieldValue(field) || undefined
}

function validate(): string {
  const missing = visibleFields.value.find((field) => field.required && !fieldValue(field).trim())
  if (missing) {
    return `参数「${missing.label}」为必填，请填写后再保存`
  }
  return ''
}

function buildParams(): ReleaseAutomationParam[] {
  return visibleFields.value
    .map((field) => ({
      pipeline_scope: field.param.pipeline_scope,
      param_key: String(field.param.param_key || field.def?.param_key || '').trim(),
      executor_param_name: String(
        field.param.executor_param_name || field.def?.executor_param_name || '',
      ).trim(),
      param_value: fieldValue(field).trim(),
      value_source: 'release_input',
    }))
    .filter((item) => item.param_value !== '')
}

watch(
  () => [props.applicationId, props.templateId, props.scopes.join(',')],
  () => {
    void loadTemplateFields()
  },
  { immediate: true },
)

watch(fixedGitRef, (value) => {
  emit('update:fixedGitRef', value)
})

watch([validate, buildParams], () => {
  submitError.value = ''
  emit('update:error', '')
  emit('update:params', buildParams())
})

defineExpose({ validate, buildParams, fields: visibleFields, loading, fixedGitRef })
</script>

<template>
  <div class="template-param-fields">
    <div v-if="loading" class="template-param-fields-hint">正在读取发布模板的参数映射…</div>
    <div v-else-if="!templateId" class="template-param-fields-hint">
      选择应用与发布模板后，这里会列出模板要求填写的参数
    </div>
    <div v-else-if="visibleFields.length === 0" class="template-param-fields-hint">
      当前模板没有需要填写的参数，自动化将按模板的固定值执行
    </div>
    <template v-else>
      <a-form-item
        v-for="field in visibleFields"
        :key="field.param.executor_param_def_id"
        class="form-item-compact"
        :required="field.required"
      >
        <template #label>
          <span class="field-label-with-hint">
            {{ field.label }}
            <span v-if="field.required" class="field-required-hint">必填</span>
            <span class="template-param-scope-tag">{{ field.scope.toUpperCase() }}</span>
          </span>
        </template>
        <a-select
          v-if="field.options.length > 0"
          class="param-value-control"
          :mode="field.multiple ? 'multiple' : 'combobox'"
          :value="choiceSelectValue(field)"
          :options="field.options"
          :disabled="disabled"
          show-search
          option-filter-prop="label"
          :placeholder="field.multiple ? '请选择（可多选）' : '请选择或手动输入'"
          allow-clear
          @change="field.multiple ? setMultiValue(field, $event as string[]) : setFieldValue(field, String($event || ''))"
        />
        <a-input
          v-else
          class="param-value-control"
          :value="fieldValue(field)"
          :disabled="disabled"
          placeholder="请输入发布值"
          allow-clear
          @update:value="setFieldValue(field, $event)"
        />
      </a-form-item>
    </template>
  </div>
</template>

<style scoped>
.template-param-fields-hint {
  padding: 6px 0 10px;
  color: var(--color-text-soft);
  font-size: 13px;
}

.field-label-with-hint {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.field-required-hint {
  color: #d4380d;
  font-size: 12px;
}

.template-param-scope-tag {
  padding: 0 6px;
  border-radius: 6px;
  background: rgba(148, 163, 184, 0.16);
  color: var(--color-text-soft);
  font-size: 11px;
  font-weight: 600;
}

.param-value-control {
  width: 100%;
}
</style>
