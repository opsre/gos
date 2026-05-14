import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'

const viewURL = new URL('../src/views/application/ApplicationListView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')

test('application release detail defaults to the newest release order environment including failed orders', () => {
  assert.match(
    source,
    /case 'deploy_failed':[\s\S]*case 'failed':[\s\S]*return 'deploy_failed'/,
    'release detail should normalize failed release orders into the failed business status bucket',
  )
  assert.match(
    source,
    /function defaultReleaseDetailSection\(card: WorkbenchCard\)[\s\S]*const latestOrderID = normalizedValue\(card\.latestOrder\?\.id\)[\s\S]*sections\.find\(\(section\) => normalizedValue\(section\.latestOrder\?\.id\) === latestOrderID\)[\s\S]*return latestSection/,
    'release detail should pick the environment containing the newest release order before falling back to env order',
  )
  assert.match(
    source,
    /return sections\.find\(\(section\) => section\.envCode === selectedEnv\) \|\| defaultReleaseDetailSection\(card\)/,
    'release detail should use the newest release order environment when no environment was selected',
  )
})
