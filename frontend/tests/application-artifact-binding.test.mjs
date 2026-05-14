import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const formURL = new URL('../src/views/application/ApplicationForm.vue', import.meta.url)
const createURL = new URL('../src/views/application/ApplicationCreateView.vue', import.meta.url)
const editURL = new URL('../src/views/application/ApplicationEditView.vue', import.meta.url)
const typeURL = new URL('../src/types/application.ts', import.meta.url)

const formSource = readFileSync(formURL, 'utf8')
const createSource = readFileSync(createURL, 'utf8')
const editSource = readFileSync(editURL, 'utf8')
const typeSource = readFileSync(typeURL, 'utf8')

test('application payload includes optional artifact repository binding fields', () => {
  assert.match(typeSource, /artifact_repository_id: string/, 'application responses should include artifact repository id')
  assert.match(typeSource, /artifact_directory: string/, 'application responses should include app artifact directory')
  assert.match(
    typeSource,
    /export interface ApplicationPayload \{[\s\S]*artifact_repository_id: string[\s\S]*artifact_directory: string[\s\S]*\}/,
    'application create and update payload should carry artifact binding fields',
  )
})

test('application form exposes optional artifact repository and directory fields', () => {
  assert.match(formSource, /artifact_repository_id: string/, 'form model should keep selected artifact repository id')
  assert.match(formSource, /artifact_directory: string/, 'form model should keep selected artifact directory')
  assert.match(formSource, /artifactRepositoryOptions\?: ArtifactRepositoryOption\[\]/, 'form should accept artifact repository options')
  assert.match(
    formSource,
    /<a-row :gutter="12" class="form-row-compact form-row-artifact-binding">[\s\S]*name="artifact_repository_id"[\s\S]*制品库[\s\S]*v-model:value="model\.artifact_repository_id"[\s\S]*:options="artifactRepositoryOptions"[\s\S]*name="artifact_directory"[\s\S]*制品路径[\s\S]*v-model:value="model\.artifact_directory"/,
    'configuration section should add a second row for optional artifact repository and app artifact path',
  )
  assert.match(
    formSource,
    /artifact_directory:\s*\[\s*\{\s*validator: validateArtifactDirectory/,
    'artifact directory should be conditionally validated when a repository is selected',
  )
})

test('application create and edit pages load artifact repositories for the shared form', () => {
  for (const [name, source] of [['create', createSource], ['edit', editSource]]) {
    assert.match(source, /listArtifactRepositories/, `${name} page should load artifact repository options`)
    assert.match(source, /const artifactRepositoryLoading = ref\(false\)/, `${name} page should keep loading state`)
    assert.match(source, /async function loadArtifactRepositoryOptions\(\)/, `${name} page should define an option loader`)
    assert.match(
      source,
      /:artifact-repository-options="artifactRepositoryOptions"[\s\S]*:artifact-repository-loading="artifactRepositoryLoading"/,
      `${name} page should pass artifact repository options and loading state to the form`,
    )
  }
  assert.match(
    editSource,
    /artifact_repository_id: app\.artifact_repository_id,[\s\S]*artifact_directory: app\.artifact_directory,/,
    'edit page should hydrate existing artifact binding into initial form values',
  )
})
