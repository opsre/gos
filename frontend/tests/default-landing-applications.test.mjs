import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const routerSource = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
const loginSource = readFileSync(new URL('../src/views/login/LoginView.vue', import.meta.url), 'utf8')
const layoutSource = readFileSync(new URL('../src/layouts/AppLayout.vue', import.meta.url), 'utf8')

test('站点根路径默认落地到应用列表', () => {
  assert.match(
    routerSource,
    /path: '\/',\s*redirect: '\/applications',/,
    '根路径应重定向到 /applications',
  )
  assert.doesNotMatch(
    routerSource,
    /redirect: '\/release-search'/,
    '根路径不应再重定向到 /release-search',
  )
})

test('登录成功后默认落地到应用列表', () => {
  assert.match(
    loginSource,
    /void router\.replace\(redirect \|\| '\/applications'\)/,
    '登录后的兜底跳转应是 /applications',
  )
  assert.doesNotMatch(
    loginSource,
    /router\.replace\(redirect \|\| '\/release-search'\)/,
    '登录后不应再默认跳 /release-search',
  )
})

test('带 redirect 参数的深链仍然优先于默认落地页', () => {
  assert.match(
    loginSource,
    /const redirect = String\(route\.query\.redirect \|\| ''\)\.trim\(\)/,
    '登录页应继续读取 redirect 查询参数',
  )
  assert.match(
    loginSource,
    /void router\.replace\(redirect \|\| '\/applications'\)/,
    'redirect 参数非空时必须优先跳回深链目标',
  )
})

test('发布搜索仍可从侧栏首页入口进入（只是不再是默认落地页）', () => {
  assert.match(
    layoutSource,
    /<a-menu-item key="release-home" @click="goToReleaseSearch">[\s\S]*?首页/,
    '侧栏「首页」入口应继续指向发布搜索',
  )
  assert.match(
    layoutSource,
    /void router\.push\('\/release-search'\)/,
    'goToReleaseSearch 应继续可用',
  )
})
