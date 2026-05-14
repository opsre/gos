import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/system/SystemSettingsView.vue', import.meta.url)
const apiURL = new URL('../src/api/system.ts', import.meta.url)
const typeURL = new URL('../src/types/system.ts', import.meta.url)

const viewSource = readFileSync(viewURL, 'utf8')
const apiSource = readFileSync(apiURL, 'utf8')
const typeSource = readFileSync(typeURL, 'utf8')

test('system settings exposes an AI model tab with diagnosis model action', () => {
  assert.match(viewSource, /key="ai-models"\s+tab="AI 模型"/, 'settings page should include an AI model tab')
  assert.match(viewSource, /setDiagnosisModel/, 'AI model tab should expose the set diagnosis model action')
  assert.match(viewSource, /unsetDiagnosisModel/, 'AI model tab should expose the unset diagnosis model action')
  assert.match(viewSource, /测试连接/, 'AI model tab should expose a test connection action')
  assert.match(viewSource, /诊断模型/, 'AI model tab should label the current diagnosis model')
  assert.match(viewSource, /取消诊断模型设置/, 'current diagnosis model should render a cancel diagnosis action')
})

test('system API contains AI model config helpers', () => {
  assert.match(apiSource, /listAIModelConfigs/, 'system API should list AI model configs')
  assert.match(apiSource, /createAIModelConfig/, 'system API should create AI model configs')
  assert.match(apiSource, /updateAIModelConfig/, 'system API should update AI model configs')
  assert.match(apiSource, /testAIModelConfig/, 'system API should test AI model configs')
  assert.match(apiSource, /setDiagnosisAIModelConfig/, 'system API should set the diagnosis model')
  assert.match(apiSource, /unsetDiagnosisAIModelConfig/, 'system API should unset the diagnosis model')
})

test('AI model types never expose returned plaintext API key', () => {
  assert.match(typeSource, /api_key_configured:\s*boolean/, 'returned AI model config should expose only key configured state')
  assert.match(typeSource, /api_key\?:\s*string/, 'payload may send an API key on create or update')
  assert.doesNotMatch(
    typeSource,
    /export interface AIModelConfig[\s\S]*api_key:\s*string/,
    'AIModelConfig response type should not contain a plaintext api_key field',
  )
})
