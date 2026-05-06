import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const apiURL = new URL('../src/api/release.ts', import.meta.url)
const source = readFileSync(apiURL, 'utf8')

test('pipeline stage log API uses extended timeout', () => {
  const match = source.match(
    /export async function getReleaseOrderPipelineStageLog\([\s\S]*?http\.get<ReleaseOrderPipelineStageLogResponse>\([\s\S]*?\{\s*timeout:\s*180_000,\s*\}/,
  )

  assert.ok(match, 'stage log loading should use a dedicated 180s timeout')
})

test('release order list API uses 30s timeout', () => {
  const match = source.match(
    /export async function listReleaseOrders\([\s\S]*?http\.get<ReleaseOrderListResponse>\(\"\/release-orders\",\s*\{\s*params,\s*timeout:\s*30_000,\s*\.\.\.config,\s*\}/,
  )

  assert.ok(match, 'release order list query should use a 30s timeout (default 10s + 20s extra)')
})
