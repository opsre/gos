import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const platformParamView = readFileSync(
  new URL('../src/views/application/PlatformParamDictView.vue', import.meta.url),
  'utf8',
)
const releaseTemplateView = readFileSync(
  new URL('../src/views/release/ReleaseTemplateView.vue', import.meta.url),
  'utf8',
)

test('platform param page treats GOS artifact path as protected builtin', () => {
  assert.match(
    platformParamView,
    /protectedBuiltinParamKeys[\s\S]*gos_artifact_path/,
    'platform param page should declare gos_artifact_path as a protected builtin key',
  )
  assert.match(
    platformParamView,
    /function isBuiltinParam[\s\S]*item\.builtin[\s\S]*protectedBuiltinParamKeys\.has/,
    'builtin display should be derived from builtin flag or protected builtin keys',
  )
  assert.match(
    platformParamView,
    /v-if="isBuiltinParam\(record\)"/,
    'table builtin icon should use the protected builtin helper',
  )
  assert.match(
    platformParamView,
    /v-if="!isBuiltinParam\(record\)"/,
    'protected builtin fields should not expose custom edit/delete actions',
  )
})

test('release template builtin source options include GOS artifact path', () => {
  assert.match(
    releaseTemplateView,
    /builtinTemplateSourceKeys[\s\S]*'gos_artifact_path'/,
    'release template should keep gos_artifact_path selectable as an internal builtin field',
  )
})

test('platform param and release template treat repo URL as a builtin field', () => {
  assert.match(
    platformParamView,
    /protectedBuiltinParamKeys[\s\S]*repo_url/,
    'platform param page should declare repo_url as a protected builtin key',
  )
  assert.match(
    releaseTemplateView,
    /builtinTemplateSourceKeys[\s\S]*'repo_url'/,
    'release template should keep repo_url selectable as an internal builtin field',
  )
})
