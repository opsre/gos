import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const loginViewURL = new URL('../src/views/login/LoginView.vue', import.meta.url)
const source = readFileSync(loginViewURL, 'utf8')

test('login card removes the duplicate eyebrow title', () => {
  assert.match(source, /<h2 class="login-title">GOS Release<\/h2>/, 'login card should keep the main product title')
  assert.doesNotMatch(source, /class="login-card-eyebrow"/, 'login card should not render the duplicate eyebrow title')
  assert.doesNotMatch(source, /\.login-card-eyebrow\b/, 'login card should not keep dead eyebrow styles')
})

test('login form hides required markers but keeps required validation', () => {
  assert.match(
    source,
    /<a-form[\s\S]*:required-mark="false"[\s\S]*class="login-form"/,
    'login form should hide the default required asterisks',
  )
  assert.match(source, /username: \[\{ required: true, message: '请输入用户名'/, 'username should remain required')
  assert.match(source, /password: \[\{ required: true, message: '请输入密码'/, 'password should remain required')
  assert.match(source, /label="登录账号"[\s\S]*placeholder="请输入用户名或姓名"/, 'login should accept an account or unique display name')
})

test('login card uses the standardized release platform footnote', () => {
  assert.match(source, /<div class="login-footnote">标准化应用发布平台<\/div>/, 'login footnote should use the requested platform copy')
  assert.doesNotMatch(source, /内部部署平台 · 用户与权限统一入口/, 'login footnote should not keep the old internal platform copy')
})
