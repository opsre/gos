import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const listViewURL = new URL('../src/views/release/ReleaseOrderListView.vue', import.meta.url)
const detailViewURL = new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url)
const listSource = readFileSync(listViewURL, 'utf8')
const detailSource = readFileSync(detailViewURL, 'utf8')

test('release order list carries current query parameters into detail navigation', () => {
  assert.match(
    listSource,
    /function buildReleaseListQuery\(\)[\s\S]*activeQuery\.application_id[\s\S]*activeQuery\.keyword[\s\S]*activeQuery\.created_at_to/,
    'list page should build detail navigation query from the current applied filters',
  )
  assert.match(
    listSource,
    /page:\s*String\(filters\.page\)[\s\S]*page_size:\s*String\(filters\.pageSize\)/,
    'list page should keep pagination when carrying query parameters into detail',
  )
  assert.match(
    listSource,
    /function toDetail\(id: string\)[\s\S]*router\.push\(\{[\s\S]*path: `\/releases\/\$\{id\}`,[\s\S]*query: buildReleaseListQuery\(\),[\s\S]*\}\)/,
    'detail navigation should preserve the list query parameters',
  )
})

test('release order detail returns to list with source query parameters', () => {
  assert.match(
    detailSource,
    /function buildReleaseListQuery\(\)[\s\S]*Object\.entries\(route\.query\)[\s\S]*key === "fast_execute"/,
    'detail page should build a release-list query and drop detail-only fast_execute',
  )
  assert.match(
    detailSource,
    /function goBack\(\)[\s\S]*router\.push\(\{[\s\S]*path: "\/releases",[\s\S]*query: buildReleaseListQuery\(\),[\s\S]*\}\)/,
    'back action should return to /releases with the original query parameters',
  )
})

test('release order list applies all supported query filters from the route', () => {
  for (const key of [
    'application_id',
    'keyword',
    'concurrent_batch_no',
    'concurrent_batch_name',
    'triggered_by',
    'env_code',
    'operation_type',
    'status',
    'trigger_type',
    'created_at_from',
    'created_at_to',
  ]) {
    assert.match(
      listSource,
      new RegExp(`routeQueryText\\("${key}"\\)`),
      `list page should read ${key} from route query`,
    )
  }
})
