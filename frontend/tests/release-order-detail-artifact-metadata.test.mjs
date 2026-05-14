import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url)
const viewSource = readFileSync(viewURL, 'utf8')
const apiURL = new URL('../src/api/release.ts', import.meta.url)
const apiSource = readFileSync(apiURL, 'utf8')
const typeURL = new URL('../src/types/release.ts', import.meta.url)
const typeSource = readFileSync(typeURL, 'utf8')

test('release API exposes artifact metadata list endpoint', () => {
  assert.match(
    typeSource,
    /export interface ReleaseOrderArtifactMetadata[\s\S]*artifact_url:\s*string[\s\S]*metadata:\s*Record<string,\s*unknown>/,
    'release types should model artifact metadata returned by the backend',
  )
  assert.match(
    typeSource,
    /export interface ReleaseOrderArtifactMetadataListResponse[\s\S]*data:\s*ReleaseOrderArtifactMetadata\[\]/,
    'release types should expose the list response shape',
  )
  assert.match(
    apiSource,
    /function listReleaseOrderArtifactMetadata[\s\S]*\/release-orders\/\$\{id\}\/artifact-metadata/,
    'release API should fetch artifact metadata from the release order endpoint',
  )
})

test('release detail loads and renders artifact metadata', () => {
  assert.match(
    viewSource,
    /listReleaseOrderArtifactMetadata/,
    'detail page should import and call the artifact metadata API',
  )
  assert.match(
    viewSource,
    /const artifactMetadata = ref<ReleaseOrderArtifactMetadata\[\]>\(\[\]\)/,
    'detail page should keep artifact metadata as explicit state',
  )
  assert.match(
    viewSource,
    /Promise\.allSettled\(\[[\s\S]*listReleaseOrderArtifactMetadata\(orderID\.value\)/,
    'detail loading should fetch artifact metadata with the other release detail data',
  )
  assert.match(
    viewSource,
    /class="detail-card detail-side-card artifact-metadata-card"[\s\S]*title="制品信息"/,
    'detail page should render an artifact metadata card',
  )
  assert.match(
    viewSource,
    /v-for="artifact in artifactMetadata"[\s\S]*:href="artifact\.artifact_url"[\s\S]*target="_blank"/,
    'artifact metadata card should render artifact download links',
  )
  assert.match(
    viewSource,
    /formatArtifactSize\(artifact\.size_bytes\)/,
    'artifact metadata card should show human-readable artifact size',
  )
})
