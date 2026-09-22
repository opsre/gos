import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const loginViewURL = new URL('../src/views/login/LoginView.vue', import.meta.url)
const source = readFileSync(loginViewURL, 'utf8')

test('login view auto-fills and submits credentials from the query string', () => {
  assert.match(
    source,
    /onMounted\(\(\) => \{[\s\S]*route\.query\.username[\s\S]*route\.query\.password[\s\S]*handleSubmit\(\)/,
    'onMounted should read username/password from the query and submit them',
  )
  assert.match(
    source,
    /delete restQuery\.username[\s\S]*delete restQuery\.password[\s\S]*router\.replace/,
    'the credentials should be stripped from the address bar right after being read',
  )
})

test('query credentials stay optional so the normal login flow is untouched', () => {
  assert.match(
    source,
    /if \(!username \|\| !password\) \{\s*return\s*\}/,
    'a login link without both values must fall back to the empty form',
  )
})
