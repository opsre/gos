<script setup lang="ts">
import { LockOutlined, UserOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance, Rule } from 'ant-design-vue/es/form'
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../../stores/auth'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const formRef = ref<FormInstance>()
const submitting = ref(false)

const formState = reactive({
  username: '',
  password: '',
})

const rules: Record<string, Rule[]> = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function handleSubmit() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  submitting.value = true
  try {
    await authStore.login(formState.username, formState.password)
    message.success('登录成功')
    const redirect = String(route.query.redirect || '').trim()
    void router.replace(redirect || '/release-search')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '登录失败，请检查用户名和密码'))
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <div class="login-background"></div>
    <div class="login-shell">
      <a-card class="login-card" :bordered="false">
        <div class="login-card-head">
          <img class="login-logo" src="/logo-mark.svg?v=20260829-b" alt="" />
          <h2 class="login-title">GOS Release</h2>
        </div>

        <a-form
          ref="formRef"
          layout="vertical"
          :model="formState"
          :rules="rules"
          :required-mark="false"
          autocomplete="off"
          class="login-form"
          @keyup.enter="handleSubmit"
        >
          <a-form-item label="用户名" name="username">
            <a-input v-model:value="formState.username" placeholder="请输入用户名">
              <template #prefix>
                <UserOutlined />
              </template>
            </a-input>
          </a-form-item>

          <a-form-item label="密码" name="password">
            <a-input-password v-model:value="formState.password" placeholder="请输入密码">
              <template #prefix>
                <LockOutlined />
              </template>
            </a-input-password>
          </a-form-item>

          <a-button type="primary" block size="large" :loading="submitting" @click="handleSubmit">登录系统</a-button>
        </a-form>

        <div class="login-footnote">标准化应用发布平台</div>
      </a-card>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  position: relative;
  display: grid;
  place-items: center;
  min-height: 100vh;
  overflow: hidden;
  background:
    radial-gradient(circle at 16% 12%, rgba(37, 99, 235, 0.22), transparent 24%),
    radial-gradient(circle at 82% 18%, rgba(14, 165, 233, 0.12), transparent 20%),
    radial-gradient(circle at 50% 88%, rgba(15, 23, 42, 0.12), transparent 24%),
    linear-gradient(135deg, #0b1424 0%, #0f172a 48%, #162033 100%);
  padding: 28px;
}

.login-background {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(120deg, rgba(59, 130, 246, 0.06), transparent 38%),
    linear-gradient(rgba(148, 163, 184, 0.06) 1px, transparent 1px),
    linear-gradient(90deg, rgba(148, 163, 184, 0.06) 1px, transparent 1px);
  background-size: auto, 32px 32px, 32px 32px;
  mask-image: radial-gradient(circle at center, rgba(0, 0, 0, 0.9), transparent 92%);
  pointer-events: none;
}

.login-shell {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 460px;
  margin: 0 auto;
}

.login-card {
  width: 100%;
  border-radius: 28px;
  border: 1px solid rgba(148, 163, 184, 0.14);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.95), rgba(248, 250, 252, 0.92)),
    rgba(255, 255, 255, 0.9);
  box-shadow:
    0 24px 60px rgba(2, 6, 23, 0.28),
    inset 0 1px 0 rgba(255, 255, 255, 0.82);
  backdrop-filter: blur(16px);
}

.login-card-head {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 12px;
  text-align: left;
}

.login-logo {
  display: block;
  flex: 0 0 auto;
  width: 46px;
  height: 46px;
  border-radius: 15px;
  box-shadow: 0 12px 26px rgba(15, 23, 42, 0.16);
}

.login-title {
  margin: 0;
  color: #0f172a;
  font-size: 38px;
  font-weight: 800;
  letter-spacing: -0.03em;
}

.login-form :deep(.ant-form-item) {
  margin-bottom: 22px;
}

.login-form :deep(.ant-form-item-label > label) {
  color: #334155;
  font-weight: 600;
}

.login-form :deep(.ant-input-affix-wrapper),
.login-form :deep(.ant-input) {
  min-height: 50px;
  border-radius: 16px;
  border-color: rgba(148, 163, 184, 0.42);
  background: rgba(248, 250, 252, 0.94);
  box-shadow: none;
}

.login-form :deep(.ant-input-affix-wrapper:hover),
.login-form :deep(.ant-input:hover) {
  border-color: rgba(37, 99, 235, 0.46);
}

.login-form :deep(.ant-input-affix-wrapper-focused),
.login-form :deep(.ant-input-affix-wrapper:focus),
.login-form :deep(.ant-input:focus) {
  border-color: rgba(29, 78, 216, 0.72);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.12);
}

.login-form :deep(.ant-input-prefix) {
  color: #64748b;
}

.login-form :deep(.ant-btn) {
  height: 50px;
  margin-top: 6px;
  border-radius: 16px;
  font-size: 16px;
  font-weight: 700;
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
  border-color: #1d4ed8;
  box-shadow: 0 18px 30px rgba(37, 99, 235, 0.22);
}

.login-form :deep(.ant-btn:hover),
.login-form :deep(.ant-btn:focus) {
  background: linear-gradient(135deg, #1e40af, #1d4ed8);
  border-color: #1e40af;
}

.login-footnote {
  margin-top: 22px;
  color: #94a3b8;
  font-size: 13px;
  text-align: center;
}

@media (max-width: 1100px) {
  .login-shell {
    max-width: 520px;
  }
}

@media (max-width: 768px) {
  .login-page {
    padding: 18px;
  }

  .showcase-mark {
    width: 54px;
    height: 54px;
    border-radius: 18px;
    font-size: 24px;
  }

  .login-title {
    font-size: 28px;
  }
}
</style>
