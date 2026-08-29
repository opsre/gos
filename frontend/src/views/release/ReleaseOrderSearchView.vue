<script setup lang="ts">
import {
  AppstoreOutlined,
  AuditOutlined,
  ArrowRightOutlined,
  ExclamationCircleFilled,
  LoadingOutlined,
  PlusOutlined,
  SearchOutlined,
  UnorderedListOutlined,
} from "@ant-design/icons-vue";
import { computed, nextTick, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { listReleaseOrders } from "../../api/release";
import { extractHTTPErrorMessage } from "../../utils/http-error";

const route = useRoute();
const router = useRouter();

const inputRef = ref<HTMLInputElement | null>(null);
const orderNo = ref("");
const querying = ref(false);
const inputFocused = ref(false);
const queryError = ref("");

const normalizedOrderNo = computed(() =>
  String(orderNo.value || "")
    .trim()
    .toUpperCase(),
);

function handleInput() {
  if (queryError.value) {
    queryError.value = "";
  }
}

async function handleSearch() {
  const keyword = normalizedOrderNo.value;
  if (!keyword) {
    queryError.value = "请输入完整发布单号";
    await nextTick();
    inputRef.value?.focus();
    return;
  }
  if (querying.value) {
    return;
  }

  querying.value = true;
  queryError.value = "";
  try {
    const response = await listReleaseOrders({
      keyword,
      page: 1,
      page_size: 20,
    });
    const matchedOrder = response.data.find(
      (item) => String(item.order_no || "").trim().toUpperCase() === keyword,
    );
    if (!matchedOrder) {
      queryError.value = `未找到发布单「${keyword}」，请检查单号是否完整`;
      return;
    }
    await router.push({
      name: "release-order-detail",
      params: { id: matchedOrder.id },
    });
  } catch (error) {
    queryError.value = extractHTTPErrorMessage(error, "发布单查询失败，请稍后重试");
  } finally {
    querying.value = false;
  }
}

function goToReleaseOrders() {
  void router.push({ name: "release-order-list" });
}

function goToCreateRelease() {
  void router.push({ name: "release-order-create" });
}

function goToApprovalWorkbench() {
  void router.push({ name: "release-approval-workbench" });
}

function goToApplications() {
  void router.push({ name: "application-list" });
}

onMounted(async () => {
  const initialOrderNo = String(route.query.order_no || "").trim();
  if (initialOrderNo) {
    orderNo.value = initialOrderNo;
  }
  await nextTick();
  inputRef.value?.focus();
});
</script>

<template>
  <section class="release-order-search-page">
    <main class="release-order-search-shell">
      <div class="release-search-wordmark">GOS Release</div>
      <h1 class="release-search-title">查找发布单</h1>
      <p class="release-search-description">
        输入发布单号，查看审批、构建与部署全过程
      </p>

      <form
        class="release-search-form"
        :class="{
          'release-search-form-focused': inputFocused,
          'release-search-form-error': Boolean(queryError),
          'release-search-form-loading': querying,
        }"
        role="search"
        @submit.prevent="handleSearch"
      >
        <SearchOutlined class="release-search-input-icon" aria-hidden="true" />
        <input
          ref="inputRef"
          v-model="orderNo"
          class="release-search-input"
          name="release_order_no"
          type="search"
          autocomplete="off"
          autocapitalize="characters"
          :spellcheck="false"
          maxlength="80"
          aria-label="发布单号"
          placeholder="请输入完整发布单号，例如 RO-20260718080037-F07B67B0"
          @input="handleInput"
          @focus="inputFocused = true"
          @blur="inputFocused = false"
          @keydown.enter.prevent="handleSearch"
        />
        <button
          class="release-search-submit"
          type="submit"
          :disabled="querying"
          aria-label="查询发布单"
        >
          <LoadingOutlined v-if="querying" spin />
          <ArrowRightOutlined v-else />
        </button>
      </form>

      <div class="release-search-feedback" aria-live="polite">
        <p v-if="queryError" class="release-search-error" role="alert">
          <ExclamationCircleFilled />
          <span>{{ queryError }}</span>
        </p>
        <p v-else class="release-search-hint">
          按 <kbd>Enter</kbd> 查询
        </p>
      </div>

      <nav class="release-search-shortcuts" aria-label="首页快捷入口">
        <button
          class="release-search-shortcut"
          type="button"
          @click="goToReleaseOrders"
        >
          <UnorderedListOutlined />
          <span>全部发布单</span>
          <ArrowRightOutlined class="release-search-shortcut-arrow" />
        </button>
        <button
          class="release-search-shortcut"
          type="button"
          @click="goToCreateRelease"
        >
          <PlusOutlined />
          <span>新建发布单</span>
          <ArrowRightOutlined class="release-search-shortcut-arrow" />
        </button>
        <button
          class="release-search-shortcut"
          type="button"
          @click="goToApprovalWorkbench"
        >
          <AuditOutlined />
          <span>审批待办</span>
          <ArrowRightOutlined class="release-search-shortcut-arrow" />
        </button>
        <button
          class="release-search-shortcut"
          type="button"
          @click="goToApplications"
        >
          <AppstoreOutlined />
          <span>我的应用</span>
          <ArrowRightOutlined class="release-search-shortcut-arrow" />
        </button>
      </nav>
    </main>
  </section>
</template>

<style scoped>
.release-order-search-page {
  position: relative;
  display: flex;
  min-height: calc(100vh - 60px);
  box-sizing: border-box;
  align-items: flex-start;
  justify-content: center;
  overflow: hidden;
  padding: 132px 28px 72px;
  color: #172033;
}

.release-order-search-shell {
  width: min(100%, 900px);
  text-align: left;
  animation: release-search-enter 0.46s cubic-bezier(0.22, 1, 0.36, 1) both;
}

.release-search-wordmark {
  color: #35445d;
  font-size: 18px;
  font-weight: 650;
  letter-spacing: -0.015em;
  line-height: 1.4;
}

.release-search-title {
  margin: 24px 0 0;
  color: #172033;
  font-size: clamp(32px, 3vw, 38px);
  font-weight: 780;
  letter-spacing: -0.035em;
  line-height: 1.2;
}

.release-search-description {
  margin: 14px 0 0;
  color: #718096;
  font-size: 16px;
  font-weight: 500;
  line-height: 1.7;
}

.release-search-form {
  display: flex;
  width: 100%;
  height: 68px;
  align-items: center;
  gap: 14px;
  box-sizing: border-box;
  margin-top: 38px;
  padding: 7px 8px 7px 24px;
  border: 1px solid rgba(148, 163, 184, 0.48);
  border-radius: 15px;
  background: #fff;
  box-shadow:
    0 14px 32px rgba(15, 23, 42, 0.055),
    0 2px 6px rgba(15, 23, 42, 0.035);
  transition:
    border-color 0.22s ease,
    box-shadow 0.22s ease,
    transform 0.22s ease;
}

.release-search-form:hover,
.release-search-form-focused {
  border-color: rgba(37, 99, 235, 0.58);
  box-shadow:
    0 16px 34px rgba(15, 23, 42, 0.065),
    0 0 0 3px rgba(59, 130, 246, 0.08);
}

.release-search-form-error {
  border-color: rgba(239, 68, 68, 0.62);
  box-shadow:
    0 14px 32px rgba(15, 23, 42, 0.055),
    0 0 0 3px rgba(239, 68, 68, 0.08);
}

.release-search-input-icon {
  flex: none;
  color: #94a3b8;
  font-size: 22px;
}

.release-search-input {
  min-width: 0;
  flex: 1;
  padding: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: #1e293b;
  font: inherit;
  font-size: 16px;
  font-weight: 550;
  line-height: 1.4;
  letter-spacing: 0.005em;
  appearance: none;
}

.release-search-input::-webkit-search-cancel-button {
  margin-right: 2px;
  cursor: pointer;
}

.release-search-input::placeholder {
  color: #a4afc0;
  font-weight: 450;
}

.release-search-submit {
  display: inline-flex;
  width: 54px;
  height: 54px;
  flex: none;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 11px;
  outline: 0;
  background: #162239;
  color: #fff;
  font-size: 20px;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.12),
    0 10px 22px rgba(15, 23, 42, 0.2);
  cursor: pointer;
  transition:
    background 0.2s ease,
    box-shadow 0.2s ease,
    transform 0.2s ease;
}

.release-search-submit:hover:not(:disabled),
.release-search-submit:focus-visible {
  background: #2563eb;
  box-shadow: 0 14px 26px rgba(37, 99, 235, 0.28);
  transform: translateX(2px);
}

.release-search-submit:disabled {
  cursor: wait;
  opacity: 0.74;
}

.release-search-feedback {
  display: flex;
  min-height: 28px;
  align-items: center;
  justify-content: center;
  margin-top: 18px;
  text-align: center;
}

.release-search-hint,
.release-search-error {
  margin: 0;
  font-size: 14px;
  font-weight: 500;
}

.release-search-hint {
  color: #96a1b4;
}

.release-search-hint kbd {
  display: inline-flex;
  min-width: 42px;
  height: 25px;
  align-items: center;
  justify-content: center;
  margin: 0 4px;
  padding: 0 7px;
  border: 1px solid rgba(148, 163, 184, 0.38);
  border-bottom-width: 2px;
  border-radius: 7px;
  background: rgba(255, 255, 255, 0.72);
  color: #6b7890;
  font: inherit;
  font-size: 12px;
}

.release-search-error {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: #dc2626;
}

.release-search-shortcuts {
  position: relative;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  margin-top: 34px;
  padding-top: 28px;
  border-top: 1px solid rgba(148, 163, 184, 0.32);
}

.release-search-shortcut {
  position: relative;
  display: flex;
  min-width: 0;
  height: 46px;
  align-items: center;
  justify-content: center;
  gap: 11px;
  padding: 0 28px;
  border: 0;
  outline: 0;
  background: transparent;
  color: #27364d;
  font-family: inherit;
  font-size: 15px;
  font-weight: 650;
  cursor: pointer;
  transition:
    color 0.18s ease,
    transform 0.18s ease;
}

.release-search-shortcut + .release-search-shortcut::before {
  content: "";
  position: absolute;
  left: 0;
  width: 1px;
  height: 32px;
  background: rgba(148, 163, 184, 0.3);
}

.release-search-shortcut :deep(.anticon) {
  flex: none;
  font-size: 18px;
}

.release-search-shortcut-arrow {
  margin-left: 4px;
  color: #3b82f6;
  font-size: 14px !important;
  transition: transform 0.18s ease;
}

.release-search-shortcut:hover,
.release-search-shortcut:focus-visible {
  color: #2563eb;
  transform: translateY(-1px);
}

.release-search-shortcut:hover .release-search-shortcut-arrow,
.release-search-shortcut:focus-visible .release-search-shortcut-arrow {
  transform: translateX(3px);
}

@keyframes release-search-enter {
  from {
    opacity: 0;
    transform: translateY(14px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 768px) {
  .release-order-search-page {
    min-height: calc(100vh - 48px);
    padding: 84px 0 64px;
  }

  .release-order-search-shell {
    width: 100%;
  }

  .release-search-wordmark {
    font-size: 16px;
  }

  .release-search-title {
    margin-top: 18px;
    font-size: 30px;
  }

  .release-search-description {
    margin-top: 12px;
    font-size: 15px;
  }

  .release-search-form {
    height: 64px;
    gap: 10px;
    margin-top: 30px;
    padding: 7px 8px 7px 19px;
  }

  .release-search-input-icon {
    font-size: 20px;
  }

  .release-search-input {
    font-size: 15px;
  }

  .release-search-submit {
    width: 48px;
    height: 48px;
    font-size: 18px;
  }

  .release-search-shortcuts {
    grid-template-columns: 1fr;
    margin-top: 28px;
    padding-top: 18px;
  }

  .release-search-shortcut {
    justify-content: flex-start;
    padding: 0 12px;
  }

  .release-search-shortcut + .release-search-shortcut::before {
    top: 0;
    right: 12px;
    left: 12px;
    width: auto;
    height: 1px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .release-order-search-shell {
    animation: none;
  }

  .release-search-form,
  .release-search-submit,
  .release-search-shortcut,
  .release-search-shortcut-arrow {
    transition: none;
  }
}
</style>
