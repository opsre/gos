<script setup lang="ts">
import { ArrowLeftOutlined } from '@ant-design/icons-vue'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()

function goBack() {
  router.back()
}
</script>

<template>
  <a-config-provider
    :locale="zhCN"
    :theme="{
      token: {
        colorPrimary: '#1e293b',
        colorPrimaryHover: '#334155',
        colorPrimaryActive: '#0f172a',
        colorInfo: '#2563eb',
        colorInfoHover: '#1d4ed8',
        colorInfoActive: '#1e40af',
        colorLink: '#334155',
        colorLinkHover: '#1e293b',
        colorSuccess: '#16a34a',
        colorWarning: '#d97706',
        colorError: '#dc2626',
        colorText: '#111827',
        colorTextSecondary: '#64748b',
        colorBorder: '#d7dee8',
        colorBgLayout: '#f3f6fb',
        colorBgContainer: '#ffffff',
        colorTextLightSolid: '#eff6ff',
        borderRadius: 12,
        fontSize: 12,
      },
    }"
  >
    <a-button
      v-if="route.meta.public && route.name !== 'login'"
      class="root-page-back-btn"
      aria-label="返回上个页面"
      @click="goBack"
    >
      <template #icon>
        <ArrowLeftOutlined />
      </template>
      返回
    </a-button>
    <router-view v-slot="{ Component, route }">
      <Transition name="root-route-switch" mode="out-in" appear>
        <component :is="Component" :key="route.matched[0]?.path || route.path" class="root-route-view" />
      </Transition>
    </router-view>
  </a-config-provider>
</template>

<style scoped>
.root-route-view {
  min-height: 100vh;
}

.root-page-back-btn.ant-btn {
  position: fixed;
  top: 28px;
  right: 28px;
  z-index: 1000;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 42px;
  padding-inline: 14px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  font-weight: 600;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

.root-page-back-btn.ant-btn:hover,
.root-page-back-btn.ant-btn:focus,
.root-page-back-btn.ant-btn:focus-visible,
.root-page-back-btn.ant-btn:active {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.root-route-switch-enter-active,
.root-route-switch-leave-active {
  transition: opacity 0.12s ease;
}

.root-route-switch-enter-from {
  opacity: 0;
}

.root-route-switch-leave-to {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .root-route-switch-enter-active,
  .root-route-switch-leave-active {
    transition: none;
  }
}

@media (max-width: 768px) {
  .root-page-back-btn.ant-btn {
    top: 16px;
    right: 16px;
  }
}
</style>
