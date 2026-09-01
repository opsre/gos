import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(
  new URL('../src/views/application/ApplicationListView.vue', import.meta.url),
  'utf8',
)

test('application card status stays in the reserved eyebrow row', () => {
  assert.match(
    source,
    /\.workbench-card-header-actions\s*\{[\s\S]*?position:\s*absolute;[\s\S]*?top:\s*0;[\s\S]*?right:\s*var\(--workbench-action-right-offset\);[\s\S]*?\}/,
    'the absolute status badge must not drop into the application title and key row',
  )
  assert.match(
    source,
    /\.workbench-app-eyebrow\s*\{[\s\S]*?padding-right:\s*150px;[\s\S]*?overflow:\s*hidden;[\s\S]*?\}/,
    'the eyebrow row must reserve horizontal space for the status badge',
  )
})

test('long application names and keys remain safely truncatable', () => {
  assert.match(
    source,
    /\.workbench-app-title\s*\{[\s\S]*?min-width:\s*0;[\s\S]*?white-space:\s*nowrap;[\s\S]*?overflow:\s*hidden;[\s\S]*?text-overflow:\s*ellipsis;[\s\S]*?\}/,
  )
  assert.match(
    source,
    /\.workbench-app-key\s*\{[\s\S]*?min-width:\s*0;[\s\S]*?white-space:\s*nowrap;[\s\S]*?overflow:\s*hidden;[\s\S]*?text-overflow:\s*ellipsis;[\s\S]*?\}/,
  )
})

test('mobile cards return the status badge to document flow', () => {
  assert.match(
    source,
    /@media \(max-width:\s*768px\)\s*\{[\s\S]*?\.workbench-app-eyebrow\s*\{[\s\S]*?padding-right:\s*0;[\s\S]*?\}[\s\S]*?\.workbench-card-header-actions\s*\{[\s\S]*?position:\s*static;[\s\S]*?\}/,
  )
})
