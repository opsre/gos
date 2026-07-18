import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/system/SystemSettingsView.vue', import.meta.url)
const typeURL = new URL('../src/types/system.ts', import.meta.url)

const viewSource = readFileSync(viewURL, 'utf8')
const typeSource = readFileSync(typeURL, 'utf8')

test('release settings edits environment configs with description text', () => {
  assert.match(typeSource, /export interface ReleaseEnvironmentConfig/, 'release settings should expose structured environment configs')
  assert.match(typeSource, /env_configs:\s*ReleaseEnvironmentConfig\[\]/, 'release settings should return environment configs')
  assert.match(viewSource, /新增环境/, 'release settings should expose an explicit add environment action')
  assert.match(viewSource, /编辑环境/, 'release settings should expose an edit environment action')
  assert.match(viewSource, /环境编码/, 'environment editor should collect the environment code')
  assert.match(viewSource, /描述文字/, 'environment editor should collect the environment description')
  assert.match(viewSource, /envConfigForm\.description/, 'environment description should be a first-class edited field')
  assert.doesNotMatch(viewSource, /mode="tags"/, 'environment options should no longer be maintained with tag input')
})
