<script setup lang="ts">
import { ReloadOutlined } from "@ant-design/icons-vue";
import { message } from "ant-design-vue";
import { computed, ref, watch } from "vue";
import { getExecutorParamDefByID } from "../../api/pipeline";
import {
  getReleaseTemplateByID,
  listReleaseOrderParams,
} from "../../api/release";
import type { ExecutorParamDef } from "../../types/pipeline";
import type {
  ReleaseOrder,
  ReleaseOrderParam,
  ReleaseTemplateParam,
  ReplayReleaseOrderPayload,
} from "../../types/release";
import { extractHTTPErrorMessage } from "../../utils/http-error";

interface SelectOption {
  label: string;
  value: string;
}

interface ReplayCDParamRow {
  id: string;
  label: string;
  description: string;
  paramKey: string;
  executorParamName: string;
  paramType: ExecutorParamDef["param_type"];
  editable: boolean;
  multiple: boolean;
  delimiter: string;
  options: SelectOption[];
  value: string;
}

const props = defineProps<{
  open: boolean;
  order: ReleaseOrder | null;
  confirmLoading?: boolean;
}>();

const emit = defineEmits<{
  cancel: [];
  confirm: [payload: ReplayReleaseOrderPayload];
}>();

const loading = ref(false);
const loadError = ref("");
const sourceParamsUnavailable = ref(false);
const overrideCDParams = ref(false);
const cdParamRows = ref<ReplayCDParamRow[]>([]);
let loadSequence = 0;

const supportsCDParamOverride = computed(
  () =>
    Boolean(props.order?.has_ci_execution) &&
    Boolean(props.order?.has_cd_execution) &&
    String(props.order?.cd_provider || "").trim() !== "",
);

const editableCDParamRows = computed(() =>
  cdParamRows.value.filter((item) => item.editable),
);

const canOverrideCDParams = computed(
  () =>
    supportsCDParamOverride.value &&
    !loading.value &&
    !loadError.value &&
    editableCDParamRows.value.length > 0,
);

function normalizeText(value: unknown) {
  return String(value ?? "").trim();
}

function paramIdentity(paramKey: string, executorParamName: string) {
  return `${normalizeText(paramKey).toLowerCase()}::${normalizeText(executorParamName).toLowerCase()}`;
}

function splitChoiceText(value: string, delimiter = ",") {
  const text = normalizeText(value);
  if (!text) {
    return [];
  }
  if (text.includes("\n") || text.includes("\r")) {
    return text
      .replace(/\r\n/g, "\n")
      .replace(/\r/g, "\n")
      .split("\n")
      .map(normalizeText)
      .filter(Boolean);
  }
  if (delimiter && text.includes(delimiter)) {
    return text.split(delimiter).map(normalizeText).filter(Boolean);
  }
  if (delimiter !== "," && text.includes(",")) {
    return text.split(",").map(normalizeText).filter(Boolean);
  }
  return [text];
}

function dedupeOptions(options: SelectOption[]) {
  const seen = new Set<string>();
  return options.filter((item) => {
    const value = normalizeText(item.value);
    if (!value || seen.has(value)) {
      return false;
    }
    item.value = value;
    item.label = normalizeText(item.label) || value;
    seen.add(value);
    return true;
  });
}

function normalizeChoiceOptions(raw: unknown): SelectOption[] {
  if (Array.isArray(raw)) {
    const options: SelectOption[] = [];
    raw.forEach((item) => {
      if (item && typeof item === "object") {
        const valueObject = item as Record<string, unknown>;
        const value = normalizeText(
          valueObject.value ??
            valueObject.id ??
            valueObject.key ??
            valueObject.name ??
            valueObject.label,
        );
        const label = normalizeText(
          valueObject.label ??
            valueObject.name ??
            valueObject.text ??
            valueObject.description ??
            value,
        );
        if (value) {
          options.push({ label: label || value, value });
        }
        return;
      }
      splitChoiceText(normalizeText(item)).forEach((value) => {
        options.push({ label: value, value });
      });
    });
    return dedupeOptions(options);
  }
  if (typeof raw === "string") {
    return dedupeOptions(
      splitChoiceText(raw).map((value) => ({ label: value, value })),
    );
  }
  if (raw && typeof raw === "object") {
    const valueObject = raw as Record<string, unknown>;
    for (const key of [
      "choiceOptions",
      "options",
      "choices",
      "choiceList",
      "values",
      "items",
      "list",
      "value",
    ]) {
      const options = normalizeChoiceOptions(valueObject[key]);
      if (options.length > 0) {
        return options;
      }
    }
  }
  return [];
}

function resolveChoiceMeta(paramDef: ExecutorParamDef | null) {
  const fallback = { options: [] as SelectOption[], multiple: false, delimiter: "," };
  if (!paramDef) {
    return fallback;
  }
  if (paramDef.param_type === "bool") {
    return {
      options: [
        { label: "true", value: "true" },
        { label: "false", value: "false" },
      ],
      multiple: false,
      delimiter: ",",
    };
  }
  const raw = normalizeText(paramDef.raw_meta);
  if (!raw) {
    return fallback;
  }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const delimiter = normalizeText(
      parsed.multiSelectDelimiter ??
        parsed.multi_select_delimiter ??
        parsed.valueDelimiter ??
        parsed.delimiter ??
        parsed.separator,
    ) || ",";
    const options = normalizeChoiceOptions(
      parsed.choiceOptions ??
        parsed.options ??
        parsed.choices ??
        parsed.choiceList ??
        parsed.values ??
        parsed.value ??
        parsed.items,
    );
    const className = normalizeText(parsed._class).toLowerCase();
    const typeName = normalizeText(
      parsed.type ?? parsed.choiceType ?? parsed.ptype,
    ).toLowerCase();
    const multiple =
      !paramDef.single_select &&
      (Boolean(parsed.multiSelect) ||
        Boolean(parsed.multi_select) ||
        Boolean(parsed.isMulti) ||
        typeName.includes("multi") ||
        typeName.includes("checkbox") ||
        className.includes("multi"));
    return { options, multiple, delimiter };
  } catch {
    return fallback;
  }
}

function findSnapshotValue(
  snapshots: ReleaseOrderParam[],
  meta: ReleaseTemplateParam,
) {
  const exactKey = paramIdentity(meta.param_key, meta.executor_param_name);
  const exact = snapshots.find(
    (item) =>
      item.pipeline_scope === "cd" &&
      paramIdentity(item.param_key, item.executor_param_name) === exactKey,
  );
  if (exact) {
    return exact.param_value;
  }
  const paramKey = normalizeText(meta.param_key).toLowerCase();
  return (
    snapshots.find(
      (item) =>
        item.pipeline_scope === "cd" &&
        normalizeText(item.param_key).toLowerCase() === paramKey,
    )?.param_value || ""
  );
}

async function loadCDParams() {
  const order = props.order;
  const sequence = ++loadSequence;
  overrideCDParams.value = false;
  cdParamRows.value = [];
  loadError.value = "";
  sourceParamsUnavailable.value = false;

  if (!props.open || !order || !supportsCDParamOverride.value) {
    loading.value = false;
    return;
  }

  loading.value = true;
  try {
    const [templateResponse, paramsResponse] = await Promise.all([
      getReleaseTemplateByID(order.template_id),
      listReleaseOrderParams(order.id).catch(() => {
        sourceParamsUnavailable.value = true;
        return null;
      }),
    ]);
    const editableTemplateParams = templateResponse.data.params
      .filter(
        (item) =>
          item.pipeline_scope === "cd" &&
          (!item.value_source || item.value_source === "release_input"),
      )
      .sort((a, b) => a.sort_no - b.sort_no);
    const definitions = await Promise.all(
      editableTemplateParams.map((item) =>
        getExecutorParamDefByID(item.executor_param_def_id)
          .then((response) => response.data)
          .catch(() => null),
      ),
    );
    if (sequence !== loadSequence || !props.open) {
      return;
    }
    const snapshots = paramsResponse?.data || [];
    cdParamRows.value = editableTemplateParams
      .map((meta, index): ReplayCDParamRow | null => {
        const paramDef = definitions[index];
        if (paramDef && !paramDef.can_view) {
          return null;
        }
        const choice = resolveChoiceMeta(paramDef);
        const snapshotValue = findSnapshotValue(snapshots, meta);
        const value = normalizeText(snapshotValue || paramDef?.default_value || "");
        const options = [...choice.options];
        splitChoiceText(value, choice.delimiter).forEach((selected) => {
          if (!options.some((option) => option.value === selected)) {
            options.push({ label: selected, value: selected });
          }
        });
        return {
          id: meta.id,
          label:
            normalizeText(meta.param_name) ||
            normalizeText(meta.param_key) ||
            normalizeText(meta.executor_param_name),
          description: normalizeText(paramDef?.description),
          paramKey: normalizeText(meta.param_key).toLowerCase(),
          executorParamName: normalizeText(meta.executor_param_name),
          paramType: paramDef?.param_type || "string",
          editable: paramDef ? paramDef.can_edit : true,
          multiple: choice.multiple,
          delimiter: choice.delimiter,
          options: dedupeOptions(options),
          value,
        };
      })
      .filter((item): item is ReplayCDParamRow => Boolean(item));
  } catch (error) {
    if (sequence === loadSequence) {
      loadError.value = extractHTTPErrorMessage(
        error,
        "CD 参数读取失败，请关闭后重试",
      );
    }
  } finally {
    if (sequence === loadSequence) {
      loading.value = false;
    }
  }
}

function selectedMultipleValues(row: ReplayCDParamRow) {
  return splitChoiceText(row.value, row.delimiter);
}

function updateMultipleValue(row: ReplayCDParamRow, value: unknown) {
  const values = Array.isArray(value) ? value.map(normalizeText).filter(Boolean) : [];
  row.value = values.join(row.delimiter || ",");
}

function handleOverrideChange(checked: boolean) {
  if (checked && !canOverrideCDParams.value) {
    return;
  }
  overrideCDParams.value = checked;
}

function handleCancel() {
  loadSequence += 1;
  emit("cancel");
}

function handleConfirm() {
  if (!overrideCDParams.value) {
    emit("confirm", { override_cd_params: false });
    return;
  }
  if (!canOverrideCDParams.value) {
    message.warning("当前没有可重新填写的 CD 参数");
    return;
  }
  for (const row of editableCDParamRows.value) {
    const value = normalizeText(row.value);
    if (!value) {
      message.error(`CD 参数“${row.label}”不能为空`);
      return;
    }
    if (row.options.length > 0) {
      const allowed = new Set(row.options.map((item) => item.value));
      const invalid = splitChoiceText(value, row.delimiter).filter(
        (item) => !allowed.has(item),
      );
      if (invalid.length > 0) {
        message.error(`CD 参数“${row.label}”包含无效选项：${invalid.join("、")}`);
        return;
      }
    }
  }
  emit("confirm", {
    override_cd_params: true,
    cd_params: editableCDParamRows.value.map((item) => ({
      pipeline_scope: "cd",
      param_key: item.paramKey,
      executor_param_name: item.executorParamName,
      param_value: normalizeText(item.value),
      value_source: "release_input",
    })),
  });
}

watch(
  () => [props.open, props.order?.id] as const,
  () => {
    void loadCDParams();
  },
  { immediate: true },
);
</script>

<template>
  <a-modal
    :open="open"
    :width="680"
    :confirm-loading="confirmLoading"
    ok-text="创建重发单"
    cancel-text="取消"
    :mask-closable="!confirmLoading"
    :closable="!confirmLoading"
    @ok="handleConfirm"
    @cancel="handleCancel"
  >
    <template #title>
      <div class="replay-modal-title">
        <span class="replay-modal-title-icon"><ReloadOutlined /></span>
        <span>重发发布单</span>
      </div>
    </template>

    <div class="replay-modal-body">
      <div class="replay-source-card">
        <span>来源发布单</span>
        <strong>{{ order?.order_no || "-" }}</strong>
        <small>{{ order?.application_name || "-" }} · {{ order?.env_code || "-" }}</small>
      </div>

      <a-alert
        v-if="!supportsCDParamOverride"
        type="info"
        show-icon
        message="本次重发将沿用原发布单参数快照"
        description="只有同时包含 CI 和 CD 的发布单，才能在重发时重新填写 CD 参数。"
      />

      <template v-else>
        <div class="override-option-card">
          <div>
            <strong>重新填写 CD 参数（可选）</strong>
            <p>默认沿用原 CD 参数；开启后仅替换下方发布输入项。</p>
          </div>
          <a-switch
            :checked="overrideCDParams"
            :disabled="!canOverrideCDParams || confirmLoading"
            @change="handleOverrideChange"
          />
        </div>

        <a-skeleton v-if="loading" active :paragraph="{ rows: 3 }" />
        <a-alert v-else-if="loadError" type="error" show-icon :message="loadError" />
        <a-alert
          v-else-if="cdParamRows.length === 0"
          type="info"
          show-icon
          message="当前 CD 管线没有可重新填写的参数，将沿用原值"
        />

        <div v-else-if="overrideCDParams" class="replay-param-panel">
          <a-alert
            v-if="sourceParamsUnavailable"
            class="snapshot-warning"
            type="warning"
            show-icon
            message="原参数快照不可见，请为可编辑参数填写新值"
          />
          <a-form layout="vertical">
            <a-row :gutter="12">
              <a-col
                v-for="row in cdParamRows"
                :key="row.id"
                :xs="24"
                :md="12"
              >
                <a-form-item required>
                  <template #label>
                    <span class="param-label">
                      {{ row.label }}
                      <small v-if="!row.editable">无编辑权限，沿用原值</small>
                    </span>
                  </template>
                  <a-select
                    v-if="row.options.length > 0 && row.multiple"
                    mode="multiple"
                    :value="selectedMultipleValues(row)"
                    :options="row.options"
                    :disabled="!row.editable || confirmLoading"
                    show-search
                    option-filter-prop="label"
                    placeholder="请选择参数值"
                    @change="updateMultipleValue(row, $event)"
                  />
                  <a-select
                    v-else-if="row.options.length > 0"
                    v-model:value="row.value"
                    :options="row.options"
                    :disabled="!row.editable || confirmLoading"
                    show-search
                    option-filter-prop="label"
                    placeholder="请选择参数值"
                  />
                  <a-input
                    v-else
                    v-model:value="row.value"
                    :disabled="!row.editable || confirmLoading"
                    :inputmode="row.paramType === 'number' ? 'decimal' : undefined"
                    placeholder="请输入新的 CD 参数值"
                    allow-clear
                  />
                  <div v-if="row.description" class="param-description">
                    {{ row.description }}
                  </div>
                </a-form-item>
              </a-col>
            </a-row>
          </a-form>
        </div>
      </template>
    </div>
  </a-modal>
</template>

<style scoped>
.replay-modal-title {
  display: flex;
  align-items: center;
  gap: 10px;
}

.replay-modal-title-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  color: #2563eb;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
  border-radius: 10px;
}

.replay-modal-body {
  display: grid;
  gap: 16px;
  padding-top: 8px;
}

.replay-source-card {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 4px 12px;
  padding: 14px 16px;
  color: #64748b;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
}

.replay-source-card strong {
  color: #172033;
}

.replay-source-card small {
  grid-column: 2;
  color: #7c899d;
}

.override-option-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 14px 16px;
  border: 1px solid #dbe6f5;
  border-radius: 12px;
}

.override-option-card strong {
  color: #172033;
}

.override-option-card p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 12px;
}

.replay-param-panel {
  padding: 16px 16px 0;
  background: #f8fbff;
  border: 1px solid #dbeafe;
  border-radius: 12px;
}

.snapshot-warning {
  margin-bottom: 14px;
}

.param-label {
  display: inline-flex;
  flex-direction: column;
  gap: 1px;
}

.param-label small,
.param-description {
  color: #94a3b8;
  font-size: 11px;
  font-weight: 400;
}

.param-description {
  margin-top: 5px;
  line-height: 1.5;
}
</style>
