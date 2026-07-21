import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const frontendHTTPSource = readFileSync(new URL('../frontend/src/api/http.ts', import.meta.url), 'utf8')
const splitNginxSource = readFileSync(new URL('../frontend/nginx.conf', import.meta.url), 'utf8')
const unifiedNginxSource = readFileSync(new URL('../docker/nginx.conf', import.meta.url), 'utf8')

test('production frontend uses same-origin API regardless of published port', () => {
  assert.match(frontendHTTPSource, /if \(import\.meta\.env\.DEV\)/)
  assert.match(frontendHTTPSource, /return window\.location\.origin/)
  assert.doesNotMatch(frontendHTTPSource, /port === ['"]5174['"]/)
})

test('both Docker frontend modes proxy login and non-HTML API requests', () => {
  for (const source of [splitNginxSource, unifiedNginxSource]) {
    assert.match(source, /if \(\$request_method != GET\)/)
    assert.match(source, /if \(\$gos_spa_request = 0\)/)
    assert.match(source, /proxy_pass http:\/\/(?:backend|127\.0\.0\.1):8081;/)
    assert.match(
      source,
      /location \^~ \/pipeline-scan\/[\s\S]*?proxy_pass http:\/\/(?:backend|127\.0\.0\.1):8081;/,
      'pipeline scan PATCH routes must bypass SPA fallback and reach the backend',
    )
  }
})
