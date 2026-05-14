import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const artifactConfigURL = new URL('../src/views/artifact/ArtifactRepositoryConfigView.vue', import.meta.url)
const artifactAPIURL = new URL('../src/api/artifact-repository.ts', import.meta.url)
const artifactTypeURL = new URL('../src/types/artifact-repository.ts', import.meta.url)
const source = readFileSync(artifactConfigURL, 'utf8')
const apiSource = readFileSync(artifactAPIURL, 'utf8')
const typeSource = readFileSync(artifactTypeURL, 'utf8')

test('artifact repository config page can add oss repositories from a form', () => {
  assert.match(typeSource, /export type ArtifactRepositoryType = 'oss'/, 'repository type should reserve a typed OSS value')
  assert.match(
    source,
    /const repositoryTypeOptions = \[\s*\{\s*label:\s*'OSS 对象存储',\s*value:\s*'oss'\s*\},\s*\]/,
    'form should expose OSS as the selectable repository type',
  )
  assert.match(source, /function openCreateRepositoryModal\(\)[\s\S]*repositoryForm\.type = 'oss'/, 'create modal should default to OSS type')
  assert.match(
    source,
    /function buildRepositoryPayload\(\): ArtifactRepositoryPayload \{[\s\S]*type: repositoryForm\.type,[\s\S]*acl: repositoryForm\.acl,[\s\S]*\}[\s\S]*async function submitRepository\(\)[\s\S]*const payload = buildRepositoryPayload\(\)[\s\S]*const response = await createArtifactRepository\(payload\)[\s\S]*artifactRepositories\.value = \[response\.data, \.\.\.artifactRepositories\.value\]/,
    'submitting the form should persist the normalized OSS repository payload and append the returned row',
  )
  assert.match(
    source,
    /<a-button class="application-toolbar-action-btn artifact-create-btn" @click="openCreateRepositoryModal">[\s\S]*新增制品库/,
    'page header should expose an add repository action',
  )
  assert.match(
    source,
    /<a-modal[\s\S]*:mask-style="repositoryModalMaskStyle"[\s\S]*:wrap-props="repositoryModalWrapProps"[\s\S]*wrap-class-name="artifact-repository-modal-wrap"[\s\S]*repositoryModalTitle/,
    'add action should open the standard repository form modal',
  )
  assert.match(source, /const repositoryModalViewportInset = ref\(0\)/, 'repository modal should track the main content viewport inset')
  assert.match(
    source,
    /const repositoryModalMaskStyle = computed\(\(\) => \(\{[\s\S]*left:\s*`\$\{repositoryModalViewportInset\.value\}px`[\s\S]*background:\s*'rgba\(15, 23, 42, 0\.08\)'[\s\S]*backdropFilter:\s*'blur\(10px\)'[\s\S]*WebkitBackdropFilter:\s*'blur\(10px\)'/,
    'repository modal should use the blurred content-area mask',
  )
  assert.match(
    source,
    /const repositoryModalWrapProps = computed\(\(\) => \(\{[\s\S]*left:\s*`\$\{repositoryModalViewportInset\.value\}px`[\s\S]*width:\s*`calc\(100% - \$\{repositoryModalViewportInset\.value\}px\)`/,
    'repository modal wrapper should be offset so the mask does not cover the app menu',
  )
  assert.match(
    source,
    /function readRepositoryModalViewportInset\(\)[\s\S]*--layout-sider-width[\s\S]*document\.querySelector\('\.app-sider'\)/,
    'repository modal should read the sidebar width using the same approach as the schedule form',
  )
  assert.match(
    source,
    /:global\(\.artifact-repository-modal-wrap \.ant-modal-content\)/,
    'repository modal shell style should target the teleported Ant modal globally',
  )
  assert.match(
    source,
    /:global\(\.artifact-repository-modal-wrap \.artifact-repository-save-btn\.ant-btn\)[\s\S]*border:\s*1px solid rgba\(96, 165, 250, 0\.42\) !important;[\s\S]*background:\s*rgba\(255, 255, 255, 0\.68\) !important;[\s\S]*color:\s*#0f172a !important;[\s\S]*backdrop-filter:\s*blur\(14px\) saturate\(135%\);/,
    'repository modal save button should use the transparent glass treatment',
  )
  assert.doesNotMatch(
    source,
    /artifact-repository-save-btn\.ant-btn[\s\S]*background:\s*#2563eb;[\s\S]*color:\s*#fff;/,
    'repository modal save button should not use a solid primary treatment',
  )
  assert.match(
    source,
    /<a-select v-model:value="repositoryForm\.type" :options="repositoryTypeOptions"/,
    'form should bind the repository type selector',
  )
  assert.match(source, /<a-input v-model:value="repositoryForm\.endpoint"/, 'form should collect OSS endpoint')
  assert.match(source, /<a-input v-model:value="repositoryForm\.bucket"/, 'form should collect OSS bucket')
  assert.match(source, /<a-input-password v-model:value="repositoryForm\.access_key_secret"/, 'form should collect OSS secret safely')
  assert.match(source, /message\.success\('制品库已新增'\)/, 'submit should tell the user the repository was added')
})

test('artifact repository config page persists repositories through backend api', () => {
  assert.match(source, /from '..\/..\/api\/artifact-repository'/, 'page should import artifact repository API functions')
  assert.match(source, /onMounted\(\(\) => \{[\s\S]*void loadArtifactRepositories\(\)[\s\S]*\}\)/, 'page should load repositories from backend on mount')
  assert.match(source, /async function loadArtifactRepositories\(\)[\s\S]*listArtifactRepositories\(/, 'page should call list API')
  assert.match(source, /await createArtifactRepository\(payload\)/, 'create mode should call create API')
  assert.match(source, /await updateArtifactRepository\(editingRepositoryID\.value, payload\)/, 'edit mode should call update API')
  assert.match(source, /await deleteArtifactRepository\(record\.id\)/, 'delete action should call delete API')
  assert.doesNotMatch(
    source,
    /id: `local-\$\{Date\.now\(\)\}`/,
    'page should not create local-only repository IDs after backend integration',
  )
})

test('artifact repository config page can test oss connectivity before saving', () => {
  assert.match(
    typeSource,
    /export interface ArtifactRepositoryConnectionTestResponse \{[\s\S]*success: boolean[\s\S]*message: string[\s\S]*\}/,
    'types should describe the connection test response',
  )
  assert.match(
    apiSource,
    /export async function testArtifactRepositoryConnection\([\s\S]*payload: ArtifactRepositoryPayload[\s\S]*\)[\s\S]*http\.post<ArtifactRepositoryConnectionTestResponse>\('\/artifact-repositories\/actions\/test-connection', payload\)/,
    'API layer should expose the backend connection test action',
  )
  assert.match(source, /testArtifactRepositoryConnection/, 'config page should import the connection test API')
  assert.match(source, /const testingRepositoryConnection = ref\(false\)/, 'page should keep connection test loading state')
  assert.match(
    source,
    /async function testRepositoryConnection\(\)[\s\S]*const payload = buildRepositoryPayload\(\)[\s\S]*await testArtifactRepositoryConnection\(payload\)[\s\S]*message\.success\(/,
    'test action should reuse the normalized form payload and show a success message',
  )
  assert.match(
    source,
    /<a-button[\s\S]*class="application-toolbar-action-btn artifact-repository-test-btn"[\s\S]*:loading="testingRepositoryConnection"[\s\S]*@click="testRepositoryConnection"[\s\S]*测试连通性/,
    'repository modal should expose a test connectivity button',
  )
})

test('artifact repository config page exposes acl, detail, edit and delete actions', () => {
  assert.match(
    source,
    /<section class="artifact-form-panel artifact-form-panel-basic">[\s\S]*<div class="artifact-form-panel-title">基础信息<\/div>[\s\S]*name="acl"[\s\S]*默认 ACL/,
    'ACL should be visible in the first basic-information panel',
  )
  assert.match(source, /const detailRepository = ref<ArtifactRepository \| null>\(null\)/, 'page should keep the selected repository for detail view')
  assert.match(source, /function openRepositoryDetail\(record: ArtifactRepository\)/, 'row action should open repository detail')
  assert.match(source, /function openEditRepositoryModal\(record: ArtifactRepository\)/, 'row action should open repository edit modal')
  assert.match(source, /function deleteRepository\(record: ArtifactRepository\)/, 'row action should delete a repository')
  assert.match(source, /editorMode\.value === 'create' \? '新增制品库' : '编辑制品库'/, 'modal title should switch between create and edit')
  assert.match(
    source,
    /artifactRepositories\.value = artifactRepositories\.value\.map\(\(item\) =>[\s\S]*item\.id === editingRepositoryID\.value/,
    'saving in edit mode should update the matching row',
  )
  assert.match(
    source,
    /<template v-else-if="column\.key === 'actions'">[\s\S]*查看基础信息[\s\S]*编辑[\s\S]*删除/,
    'table should expose detail, edit and delete row actions',
  )
  assert.match(source, /<a-drawer[\s\S]*title="制品库基础信息"[\s\S]*detailRepository/, 'detail drawer should display repository base information')
})

test('artifact repository acl selector uses a light selection style', () => {
  assert.doesNotMatch(source, /button-style="solid"/, 'ACL selector should not use dark solid radio buttons')
  assert.match(
    source,
    /\.artifact-acl-radio\s+:deep\(\.ant-radio-button-wrapper-checked:not\(\.ant-radio-button-wrapper-disabled\)\)\s*\{[\s\S]*background:\s*rgba\(239,\s*246,\s*255,\s*0\.9\)/,
    'checked ACL option should use a light blue background',
  )
  assert.doesNotMatch(
    source,
    /\.artifact-acl-radio[\s\S]*background:\s*#1677ff|\.artifact-acl-radio[\s\S]*color:\s*#fff/,
    'ACL selector should not introduce dark Ant primary selected colors',
  )
})
