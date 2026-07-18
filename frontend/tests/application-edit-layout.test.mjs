import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const editViewURL = new URL('../src/views/application/ApplicationEditView.vue', import.meta.url)
const formURL = new URL('../src/views/application/ApplicationForm.vue', import.meta.url)
const editSource = readFileSync(editViewURL, 'utf8')
const formSource = readFileSync(formURL, 'utf8')

function extractStyleRule(source, selector) {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escapedSelector}\\s*\\{([\\s\\S]*?)\\n\\}`, 'm'))
  assert.ok(match, `expected to find style rule for ${selector}`)
  return match[1]
}

test('application edit page renders the form without a background card', () => {
  const sideCardRule = extractStyleRule(editSource, '.create-side-card')

  assert.match(
    editSource,
    /class="page-wrapper application-edit-page"/,
    'application edit page should define its own scope',
  )
  assert.doesNotMatch(editSource, /class="create-main-card"/, 'application edit form should not be wrapped in a background card')
  assert.doesNotMatch(editSource, /\.create-main-card/, 'application edit page should remove the main form card styles')
  assert.match(
    editSource,
    /<div class="create-main">\s*<ApplicationForm/,
    'application edit form should sit directly inside the main column',
  )
  assert.match(
    sideCardRule,
    /background:\s*var\(--pipeline-binding-surface-background\)\s*;/,
    'application edit sidebar cards should keep the shared light surface background',
  )
  assert.match(
    sideCardRule,
    /border:\s*1px solid var\(--pipeline-binding-surface-border\)\s*;/,
    'application edit sidebar cards should keep the shared border tone',
  )
  assert.match(
    sideCardRule,
    /box-shadow:\s*var\(--pipeline-binding-surface-shadow\)\s*;/,
    'application edit sidebar cards should keep the shared shadow depth',
  )
})

test('application edit page returns to the list after detail page removal', () => {
  assert.match(
    editSource,
    /message\.success\('应用更新成功'\)[\s\S]*router\.push\('\/applications'\)/,
    'application edit save should return to the application list because detail page is removed',
  )
  assert.doesNotMatch(
    editSource,
    /router\.push\(`\/applications\/\$\{applicationId\.value\}`\)/,
    'application edit page should not navigate to the removed detail page',
  )
})

test('application edit page keeps application key readonly', () => {
  assert.match(
    formSource,
    /keyReadonly\?: boolean/,
    'shared application form should expose a key readonly switch',
  )
  assert.match(
    formSource,
    /<a-input[\s\S]*v-model:value="model\.key"[\s\S]*:readonly="keyReadonly"/,
    'application key input should support readonly mode while keeping the value in the payload',
  )
  assert.match(
    editSource,
    /<ApplicationForm[\s\S]*key-readonly/,
    'application edit page should render the shared form with application key readonly',
  )
})
