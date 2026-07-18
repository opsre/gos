import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const releaseTypesURL = new URL('../src/types/release.ts', import.meta.url)
const releaseApiURL = new URL('../src/api/release.ts', import.meta.url)
const createViewURL = new URL('../src/views/release/ReleaseOrderCreateView.vue', import.meta.url)

test('release frontend contract does not expose deprecated son_service', () => {
  const sources = [
    ['release types', readFileSync(releaseTypesURL, 'utf8')],
    ['release api', readFileSync(releaseApiURL, 'utf8')],
    ['release create view', readFileSync(createViewURL, 'utf8')],
  ]

  for (const [name, source] of sources) {
    assert.doesNotMatch(source, /son_service|SonService/, `${name} should not reference deprecated son_service`)
  }
})
