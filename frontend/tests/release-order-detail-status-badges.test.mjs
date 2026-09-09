import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const viewURL = new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')

function extractStyleRule(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\n\\}`))
  assert.ok(match, `expected to find style rule for ${selector}`)
  return match[1]
}

test('release detail status badges match the release list visual system', () => {
  assert.match(source, /function statusIconForTone\(toneClass: string\)/)
  assert.match(
    source,
    /:is="statusIconForTone\(statusToneClass\(stage\.status\)\)"/,
    'pipeline stages should pair status text with a semantic icon',
  )
  assert.match(
    source,
    /:is="statusIconForTone\(statusToneClass\(unit\.execution\.status\)\)"/,
    'execution units should pair status text with a semantic icon',
  )

  const badgeRule = extractStyleRule('.status-tag')
  assert.match(badgeRule, /min-height:\s*26px/)
  assert.match(badgeRule, /border-radius:\s*8px/)
  assert.match(badgeRule, /padding:\s*3px 9px/)

  assert.match(extractStyleRule('.status-pill-success'), /background:\s*#ecfdf3/)
  assert.match(extractStyleRule('.status-pill-running'), /background:\s*#eff6ff/)
  assert.match(extractStyleRule('.status-pill-failed'), /background:\s*#fef2f2/)
  assert.match(extractStyleRule('.status-pill-pending'), /background:\s*#fff7ed/)
})

test('release spotlight uses the same flat status surfaces', () => {
  const spotlightRule = extractStyleRule('.release-spotlight')
  assert.match(spotlightRule, /border-radius:\s*16px/)
  assert.doesNotMatch(spotlightRule, /radial-gradient|linear-gradient/)

  assert.match(extractStyleRule('.release-spotlight-running'), /background:\s*#eff6ff/)
  assert.match(
    source,
    /'release-spotlight-meta'[\s\S]*statusToneClass\(currentBusinessStatus\)[\s\S]*statusIconForTone/,
  )
})
