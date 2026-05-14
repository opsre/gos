import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/artifact/ArtifactCenterView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')
const artifactApiURL = new URL('../src/api/artifact.ts', import.meta.url)
const artifactApiSource = readFileSync(artifactApiURL, 'utf8')
const artifactTypeURL = new URL('../src/types/artifact.ts', import.meta.url)
const artifactTypeSource = readFileSync(artifactTypeURL, 'utf8')

function cssBlock(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{[\\s\\S]*?\\n\\}`))
  assert.ok(match, `missing CSS block for ${selector}`)
  return match[0]
}

test('artifact center toolbar stages multi-condition query values before search', () => {
  assert.match(
    source,
    /const activeFilters = reactive\(\{[\s\S]*repository_id:\s*'',[\s\S]*project_id:\s*'',[\s\S]*application_id:\s*'',[\s\S]*pipeline_scope:\s*'',[\s\S]*\}\)/,
    'toolbar should keep an active query separate from editable filter values',
  )
  assert.match(
    source,
    /const canQueryArtifacts = computed\(\(\) => Boolean\(activeFilters\.repository_id\)\)/,
    'table visibility should follow the submitted repository filter',
  )
  assert.match(
    source,
    /repository_id:\s*activeFilters\.repository_id,[\s\S]*project_id:\s*activeFilters\.project_id \|\| undefined,[\s\S]*application_id:\s*activeFilters\.application_id \|\| undefined,[\s\S]*pipeline_scope:\s*activeFilters\.pipeline_scope \|\| undefined/,
    'artifact query should use submitted filters rather than draft filters',
  )
  assert.match(
    source,
    /function searchArtifacts\(\)[\s\S]*activeFilters\.repository_id = filters\.repository_id[\s\S]*activeFilters\.project_id = filters\.project_id[\s\S]*activeFilters\.application_id = filters\.application_id[\s\S]*activeFilters\.pipeline_scope = filters\.pipeline_scope[\s\S]*void loadArtifacts\(\)/,
    'clicking query should commit draft filters and then load artifacts',
  )
  assert.doesNotMatch(source, /@change="handleRepositoryChange"/, 'repository changes should not immediately query')
  assert.doesNotMatch(source, /@change="handleApplicationChange"/, 'application changes should not immediately query')
  assert.doesNotMatch(source, />重置<\/a-button>/, 'toolbar should not render the reset action')
  assert.match(source, /class="artifact-toolbar-query-btn"[\s\S]*SearchOutlined[\s\S]*查询/, 'toolbar should render a query button')
})

test('artifact center defaults to the first artifact repository', () => {
  assert.match(
    source,
    /const firstRepositoryID = repositoryOptions\.value\[0\]\?\.value/,
    'repository loading should read the first available repository option',
  )
  assert.match(
    source,
    /if \(!filters\.repository_id && firstRepositoryID\) \{[\s\S]*filters\.repository_id = firstRepositoryID[\s\S]*activeFilters\.repository_id = firstRepositoryID[\s\S]*void loadArtifacts\(\)[\s\S]*\}/,
    'first repository should become both the draft and submitted repository filter',
  )
})

test('artifact center toolbar uses compact file-manager controls', () => {
  assert.match(source, /<div class="page-header">[\s\S]*<div class="page-title">制品目录<\/div>/, 'page should keep the artifact catalog title in the standard page header')
  const toolbarIndex = source.indexOf('<div class="artifact-center-toolbar">')
  const windowIndex = source.indexOf('<section class="artifact-window">')
  const windowToolbarIndex = source.indexOf('<div class="artifact-window-toolbar">')
  const browserIndex = source.indexOf('<div v-if="canQueryArtifacts" class="artifact-browser">')
  assert.ok(toolbarIndex > -1 && toolbarIndex < windowIndex, 'query controls should sit in the page header above the file window')
  assert.ok(!source.slice(windowToolbarIndex, browserIndex).includes('artifact-center-toolbar'), 'file window toolbar should only keep directory navigation controls')
  assert.match(cssBlock('.page-title'), /font-size:\s*24px;/, 'page title should match the standard page heading size')
  assert.match(cssBlock('.page-header'), /align-items:\s*center;/, 'page header should vertically align the title and query controls')
  assert.doesNotMatch(source, /artifact-page-header|artifact-page-title/, 'artifact page should not use custom title positioning')
  assert.match(cssBlock('.artifact-window'), /background:\s*rgba\(255, 255, 255, 0\.72\);/, 'folder view should use the GOS light glass surface')
  assert.match(cssBlock('.artifact-window'), /border-radius:\s*18px;/, 'folder view shell should match GOS rounded surfaces')
  const toolbarBlock = cssBlock('.artifact-center-toolbar')
  assert.doesNotMatch(toolbarBlock, /padding:/, 'toolbar container should not add card padding')
  assert.doesNotMatch(toolbarBlock, /border:/, 'toolbar container should not draw a card border')
  assert.doesNotMatch(toolbarBlock, /background:/, 'toolbar container should not draw a card background')
  assert.doesNotMatch(toolbarBlock, /box-shadow:/, 'toolbar container should not draw a card shadow')
  assert.match(toolbarBlock, /justify-content:\s*flex-end;/, 'header query controls should align to the right side')
  assert.match(source, /\.artifact-repository-filter\s*\{\s*width:\s*220px;/, 'repository filter should use the file-manager toolbar width')
  assert.match(source, /\.artifact-filter\s*\{\s*width:\s*150px;/, 'project and application filters should use compact toolbar width')
  assert.match(source, /\.artifact-scope-filter\s*\{\s*width:\s*140px;/, 'scope filter should use compact toolbar width')
  assert.match(
    source,
    /:deep\(\.artifact-toolbar-select\.ant-select \.ant-select-selector\)[\s\S]*height:\s*42px !important;[\s\S]*border-radius:\s*16px !important;[\s\S]*background:\s*rgba\(255, 255, 255, 0\.68\) !important;/,
    'select controls should carry the GOS glass toolbar styling',
  )
  assert.match(
    source,
    /:deep\(\.artifact-toolbar-query-btn\.ant-btn\)\s*\{[\s\S]*height:\s*42px;[\s\S]*border-radius:\s*16px;[\s\S]*background:\s*rgba\(255, 255, 255, 0\.68\) !important;/,
    'query button should match the GOS toolbar controls',
  )
  assert.match(
    source,
    /:deep\(\.artifact-toolbar-add-btn\.ant-btn\)\s*\{[\s\S]*height:\s*42px;[\s\S]*border-radius:\s*16px;[\s\S]*background:\s*rgba\(255, 255, 255, 0\.68\) !important;[\s\S]*color:\s*#0f172a !important;/,
    'manual add button should use the same transparent glass treatment as the query button',
  )
  assert.doesNotMatch(
    cssBlock(':deep(.artifact-toolbar-query-btn.ant-btn)'),
    /linear-gradient/,
    'query button should not use a separate gradient treatment',
  )
  assert.doesNotMatch(cssBlock(':deep(.artifact-toolbar-add-btn.ant-btn)'), /background:\s*#2563eb|background:\s*#1d4ed8|color:\s*#fff/, 'manual add button should not use a solid primary treatment')
})

test('artifact center does not render selected tree path breadcrumbs', () => {
  assert.doesNotMatch(source, /全部制品/, 'the all-artifacts breadcrumb label should be removed')
  assert.doesNotMatch(source, /breadcrumbItems/, 'selected tree path state should not be exposed for rendering')
  assert.doesNotMatch(source, /class="artifact-breadcrumb"/, 'selected tree path container should not render')
  assert.doesNotMatch(source, /\.artifact-breadcrumb/, 'selected tree path styles should be removed')
})

test('artifact center uses a file-manager grid instead of a table', () => {
  assert.match(source, /class="artifact-window"/, 'artifact page should render as a file-manager window')
  assert.doesNotMatch(source, /artifact-window-tabs|artifact-window-tab|制品库视图|AppstoreOutlined/, 'artifact page should not render the removed window tab row')
  assert.match(source, /class="artifact-window-toolbar"/, 'file-manager shell should include a compact toolbar')
  assert.match(source, /class="artifact-file-grid"/, 'files should render in an icon grid')
  assert.match(source, /const explorerPanelTitle = computed\(\(\) => currentTreeNode\.value\?\.title \|\| '根目录'\)/, 'file panel should expose the current directory title')
  assert.match(source, /class="artifact-file-panel-title"[\s\S]*目录[\s\S]*explorerPanelTitle/, 'file panel should render a compact directory title')
  assert.match(source, /v-for="directory in explorerDirectories"/, 'grid should show directory tiles')
  assert.match(source, /v-for="record in explorerFiles"/, 'grid should show file tiles')
  assert.match(
    source,
    /const explorerDirectories = computed\(\(\) => \{[\s\S]*currentTreeNode\.value \? currentTreeNode\.value\.children \|\| \[\] : artifactTreeData\.value/,
    'leaf directories should expose an empty directory list instead of falling back to root directories',
  )
  assert.match(
    source,
    /const explorerFiles = computed\(\(\) => \{[\s\S]*return explorerDirectories\.value\.length > 0 \? \[\] : visibleArtifacts\.value[\s\S]*\}\)/,
    'file tiles should only render when the current level has no child directories',
  )
  assert.doesNotMatch(
    source,
    /if \(!selectedTreeKeys\.value\[0\]\)[\s\S]*return visibleArtifacts\.value/,
    'root directory should not mix all package files with project folders',
  )
  assert.match(
    source,
    /const canManualAddArtifact = computed\(\(\) => canQueryArtifacts\.value && Boolean\(currentTreeNode\.value\) && explorerDirectories\.value\.length === 0\)/,
    'manual add should only be enabled at the final actual directory level',
  )
  assert.doesNotMatch(source, /<a-table/, 'file-manager view should not use the old table')
  assert.doesNotMatch(source, /const columns:/, 'file-manager view should not keep table column config')
  assert.doesNotMatch(source, /:scroll="\{ x: 1300 \}"/, 'file-manager view should not force a horizontal table scrollbar')
  assert.match(
    source,
    /\.artifact-file-grid\s*\{[\s\S]*grid-template-columns:\s*repeat\(auto-fill, minmax\(132px, 1fr\)\)/,
    'file grid should use responsive icon tiles',
  )
  assert.match(
    source,
    /class="artifact-file-tile"[\s\S]*@click="openDetail\(record\)"[\s\S]*@keyup\.enter="openDetail\(record\)"/,
    'clicking a file tile should open the artifact detail drawer',
  )
  assert.match(source, /const visibleFileActionID = ref\(''\)/, 'file tile floating actions should be controlled by right-click state')
  assert.match(
    source,
    /:class="\{ 'artifact-file-tile-actions-visible': visibleFileActionID === record\.id \}"[\s\S]*@contextmenu\.prevent\.stop="showFileActions\(record\)"/,
    'right-clicking a file tile should reveal its floating actions',
  )
  assert.match(source, /<div class="artifact-center-page" @click="hideFileActions">/, 'clicking outside a right-clicked file tile should restore the normal file tile state')
  assert.match(
    source,
    /function showFileActions\(record: ReleaseOrderArtifactMetadataSummary\)[\s\S]*visibleFileActionID\.value = record\.id \|\| ''/,
    'right-click handler should mark the selected file action strip as visible',
  )
  assert.match(source, /class="artifact-file-identity"[\s\S]*artifact-file-glyph[\s\S]*artifact-tile-name/, 'file icon and name should be grouped as replaceable file identity content')
  assert.doesNotMatch(source, /@dblclick="openDetail\(record\)"/, 'file tiles should no longer require a double-click to open detail')
  assert.doesNotMatch(source, /@click\.stop="openDetail\(record\)"/, 'file tiles should not render a separate detail action button')
  assert.doesNotMatch(source, /EyeOutlined/, 'detail icon should not be imported after removing the detail action button')
  assert.doesNotMatch(source, />\s*详情\s*<\/a-button>/, 'file tiles should not render a detail action button')
  assert.doesNotMatch(source, /class="artifact-file-detail"/, 'file tiles should not render pipeline, version, or build metadata below the name')
  assert.doesNotMatch(source, /\.artifact-file-detail/, 'removed file tile metadata should not keep unused styles')
  assert.match(
    source,
    /\.artifact-directory-tile,[\s\S]*\.artifact-file-tile\s*\{[\s\S]*border-radius:\s*16px;[\s\S]*background:\s*#fff;/,
    'folder and file tiles should use GOS light card styling',
  )
  const tileBlock = source.match(/\.artifact-directory-tile,\n\.artifact-file-tile\s*\{[\s\S]*?\n\}/)
  assert.ok(tileBlock, 'folder and file tile style block should exist')
  assert.match(tileBlock[0], /position:\s*relative;/, 'folder and file tiles should contain floating actions without affecting layout')
  assert.match(tileBlock[0], /align-self:\s*start;/, 'folder and file tiles should not stretch to leave large empty space')
  assert.match(tileBlock[0], /justify-content:\s*center;/, 'folder and file tile contents should sit vertically centered instead of hugging the top edge')
  assert.match(
    source,
    /\.artifact-file-tile-actions-visible \.artifact-file-identity\s*\{[\s\S]*display:\s*none;/,
    'right-click selected file tile should hide the original file icon and name',
  )
  const actionBlock = cssBlock('.artifact-file-actions')
  assert.match(actionBlock, /display:\s*none;/, 'file actions should be hidden until right-click replaces the file content')
  assert.match(actionBlock, /min-height:\s*96px;/, 'replacement actions should occupy the file identity area')
  assert.doesNotMatch(actionBlock, /position:\s*absolute|bottom:|margin-top:|opacity:/, 'replacement actions should not float over or reserve bottom whitespace')
  assert.match(
    source,
    /:deep\(\.artifact-file-action-btn\.ant-btn-dangerous\)\s*\{[\s\S]*color:\s*#dc2626 !important;/,
    'manual file delete action should use red text',
  )
  assert.doesNotMatch(source, /\.artifact-file-tile:hover \.artifact-file-actions/, 'file actions should not appear on hover')
  assert.match(
    source,
    /\.artifact-file-tile-actions-visible \.artifact-file-actions\s*\{[\s\S]*display:\s*flex;[\s\S]*pointer-events:\s*auto;/,
    'right-click selected file tile should replace the file icon and name with action buttons',
  )
})

test('artifact center directory panel uses the GOS sidebar treatment', () => {
  const browserBlock = cssBlock('.artifact-browser')
  assert.match(browserBlock, /grid-template-columns:\s*300px minmax\(0, 1fr\);/, 'file manager should use a fixed sidebar and flexible content area')
  assert.match(browserBlock, /background:\s*#fff;/, 'file manager body should use a light GOS content surface')
  const treePanelBlock = cssBlock('.artifact-tree-panel')
  assert.match(treePanelBlock, /background:\s*linear-gradient\(180deg, #f8fafc, #fff\);/, 'directory panel should use the light GOS sidebar surface')
  assert.match(treePanelBlock, /color:\s*#0f172a;/, 'directory sidebar text should use the standard dark text color')
  assert.match(
    source,
    /:deep\(\.artifact-tree\.ant-tree\)[\s\S]*background:\s*transparent/,
    'directory tree should inherit the sidebar surface',
  )
})

test('artifact center can manually add files into the file list', () => {
  const submitManualArtifactBlock = source.match(/async function submitManualArtifact\(\)[\s\S]*?\n}\n\nfunction openDetail/)
  assert.ok(submitManualArtifactBlock, 'manual add submit function should exist')

  assert.match(
    artifactTypeSource,
    /export interface ReleaseOrderArtifactMetadataPayload[\s\S]*artifact_name:\s*string[\s\S]*artifact_url:\s*string/,
    'artifact types should expose a manual artifact metadata payload',
  )
  assert.match(
    artifactApiSource,
    /function recordReleaseOrderArtifactMetadata\([\s\S]*releaseOrderID:\s*string,[\s\S]*payload:\s*ReleaseOrderArtifactMetadataPayload[\s\S]*\/release-orders\/\$\{id\}\/artifact-metadata/,
    'artifact API should post manual file metadata to the release order artifact endpoint',
  )
  assert.match(
    artifactApiSource,
    /function deleteReleaseOrderArtifactMetadata\([\s\S]*releaseOrderID:\s*string,[\s\S]*artifactID:\s*string[\s\S]*http\.delete\([\s\S]*\/release-orders\/\$\{releaseID\}\/artifact-metadata\/\$\{id\}/,
    'artifact API should expose a delete endpoint for manual artifact metadata',
  )
  assert.doesNotMatch(source, /import \{ listReleaseOrders \} from '..\/..\/api\/release'/, 'manual add should not browse unrelated release orders')
  assert.match(source, /PlusOutlined/, 'manual add action should use an add icon')
  assert.match(source, /const manualArtifactVisible = ref\(false\)/, 'manual add modal should have visible state')
  assert.match(
    source,
    /function inferReleaseContextFromArtifacts\(items: ReleaseOrderArtifactMetadataSummary\[\]\)[\s\S]*new Set\([\s\S]*release_order_id[\s\S]*releaseOrderIDs\.size !== 1[\s\S]*return null[\s\S]*return buildReleaseContext/,
    'manual add should infer the current release order from the visible list when it is unique',
  )
  assert.match(
    source,
    /const manualArtifactForm = reactive\([\s\S]*release_order_id:\s*'',[\s\S]*artifact_name:\s*'',[\s\S]*artifact_url:\s*'',[\s\S]*pipeline_scope:\s*'ci'/,
    'manual add form should capture release order, file name, URL, and execution unit',
  )
  assert.match(
    source,
    /const selectedReleaseContext = computed\(\(\) => \{[\s\S]*return inferReleaseContextFromArtifacts\(visibleArtifacts\.value\)[\s\S]*\}\)/,
    'manual add should fall back to the current visible file list instead of requiring a directory click',
  )
  assert.match(
    source,
    /function openManualArtifactModal\(\)[\s\S]*const context = selectedReleaseContext\.value[\s\S]*message\.warning\('请先筛选到单个发布单后再手动添加'\)[\s\S]*manualArtifactForm\.release_order_id = context\.release_order_id[\s\S]*manualArtifactVisible\.value = true/,
    'manual add action should use the inferred current release order and warn only when the list is ambiguous',
  )
  assert.match(
    source,
    /async function submitManualArtifact\(\)[\s\S]*recordReleaseOrderArtifactMetadata\(manualArtifactForm\.release_order_id,[\s\S]*artifact_name:\s*manualArtifactForm\.artifact_name\.trim\(\),[\s\S]*artifact_url:\s*manualArtifactForm\.artifact_url\.trim\(\),[\s\S]*void loadArtifacts\(\)/,
    'manual add submit should persist the file metadata and refresh the list',
  )
  assert.match(source, /metadata:\s*\{ source:\s*'manual' \}/, 'manual add should mark the artifact metadata as manually added')
  assert.match(
    source,
    /function isManualArtifact\(record: ReleaseOrderArtifactMetadataSummary\)[\s\S]*!String\(record\.execution_id \|\| ''\)\.trim\(\)/,
    'manual artifacts should be identified by the absence of an execution id',
  )
  assert.match(
    source,
    /async function deleteManualArtifact\(record: ReleaseOrderArtifactMetadataSummary\)[\s\S]*deleteReleaseOrderArtifactMetadata\(record\.release_order_id, record\.id\)[\s\S]*message\.success\('制品已删除'\)[\s\S]*void loadArtifacts\(\)/,
    'manual artifact delete action should call the delete API and refresh the current list',
  )
  assert.match(
    source,
    /v-if="isManualArtifact\(record\)"[\s\S]*DeleteOutlined[\s\S]*删除/,
    'file tiles should only show delete for manually added artifacts',
  )
  assert.doesNotMatch(
    submitManualArtifactBlock[0],
    /selectedTreeKeys\.value = \[\]/,
    'manual add should keep the current release-order directory selected after saving',
  )
  assert.match(source, /v-if="canManualAddArtifact"[\s\S]*class="artifact-toolbar-add-btn"[\s\S]*手动添加/, 'manual add should only render at the final real directory level')
  assert.match(
    source,
    /function openManualArtifactModal\(\)[\s\S]*if \(!canManualAddArtifact\.value\)[\s\S]*message\.warning\('请进入最后一级目录后再手动添加'\)/,
    'manual add should guard against non-final simulated directories',
  )
  assert.match(source, /wrap-class-name="manual-artifact-modal-wrap"/, 'manual add modal should use the documented modal shell class')
  assert.match(
    source,
    /const manualArtifactMaskStyle = computed\(\(\) => \(\{[\s\S]*background:\s*'rgba\(15, 23, 42, 0\.08\)'[\s\S]*backdropFilter:\s*'blur\(10px\)'[\s\S]*WebkitBackdropFilter:\s*'blur\(10px\)'/,
    'manual add modal should use the documented blurred background mask',
  )
  assert.match(source, /const manualArtifactViewportInset = ref\(0\)/, 'manual add modal should track the main content viewport inset')
  assert.match(
    source,
    /const manualArtifactWrapProps = computed\(\(\) => \(\{[\s\S]*left:\s*`\$\{manualArtifactViewportInset\.value\}px`[\s\S]*width:\s*`calc\(100% - \$\{manualArtifactViewportInset\.value\}px\)`/,
    'manual add modal wrapper should be offset so the mask does not cover the app menu',
  )
  assert.match(source, /:wrap-props="manualArtifactWrapProps"/, 'manual add modal should apply the offset wrapper props')
  assert.match(
    source,
    /function readManualArtifactViewportInset\(\)[\s\S]*--layout-sider-width[\s\S]*document\.querySelector\('\.app-sider'\)/,
    'manual add modal should read the sidebar width using the same approach as the schedule form',
  )
  assert.match(
    source,
    /<template #title>[\s\S]*class="manual-artifact-modal-titlebar"[\s\S]*class="manual-artifact-modal-title"[\s\S]*手动添加文件[\s\S]*manual-artifact-modal-save-btn/,
    'manual add modal should use a custom glass titlebar with the save action',
  )
  assert.match(
    source,
    /\.manual-artifact-modal-save-btn\.ant-btn\s*\{[\s\S]*border:\s*1px solid rgba\(96, 165, 250, 0\.42\) !important;[\s\S]*background:\s*rgba\(255, 255, 255, 0\.68\) !important;[\s\S]*color:\s*#0f172a !important;[\s\S]*backdrop-filter:\s*blur\(14px\) saturate\(135%\);/,
    'manual add save button should use the transparent glass treatment',
  )
  assert.doesNotMatch(
    source,
    /\.manual-artifact-modal-save-btn\.ant-btn\s*\{[\s\S]*background:\s*#2563eb;[\s\S]*color:\s*#fff;/,
    'manual add save button should not use a solid primary treatment',
  )
  assert.match(
    source,
    /:global\(\.manual-artifact-modal-wrap \.ant-modal-content\)/,
    'manual add modal shell style should target the teleported Ant modal globally',
  )
  assert.match(
    source,
    /class="manual-artifact-context-card"[\s\S]*当前目录[\s\S]*manualArtifactContextText/,
    'manual add modal should show the current directory in a styled context card',
  )
  assert.match(
    source,
    /class="manual-artifact-form-section"[\s\S]*基础信息[\s\S]*class="manual-artifact-form-section"[\s\S]*扩展信息/,
    'manual add form should be split into styled sections',
  )
  assert.doesNotMatch(source, /manualReleaseOrderOptions/, 'manual add modal should not render release-order choices')
})

test('artifact center removes copy actions', () => {
  assert.doesNotMatch(source, /CopyOutlined/, 'copy icon should not be imported')
  assert.doesNotMatch(source, /copyArtifactURL/, 'copy action should be removed')
  assert.doesNotMatch(source, /navigator\.clipboard/, 'copy action should not touch the clipboard')
  assert.doesNotMatch(source, />\s*复制\s*</, 'row action should not render Copy')
})

test('artifact detail drawer uses release-order navigation instead of download', () => {
  assert.match(source, /import \{ useRouter \} from 'vue-router'/, 'detail drawer should be able to navigate to release orders')
  assert.match(source, /const router = useRouter\(\)/, 'release order navigation should use the current router')
  assert.match(
    source,
    /function viewReleaseOrder\(record: ReleaseOrderArtifactMetadataSummary\)[\s\S]*if \(!record\.release_order_id\)[\s\S]*router\.push\(`\/releases\/\$\{record\.release_order_id\}`\)/,
    'detail drawer should navigate to the related release order',
  )
  assert.match(
    source,
    /<a-descriptions-item label="发布">[\s\S]*class="artifact-release-link"[\s\S]*@click="viewReleaseOrder\(selectedArtifact\)"[\s\S]*selectedArtifact\.release_display_name \|\| buildReleaseDisplayName\(selectedArtifact\)/,
    'release value itself should navigate to the related release order',
  )
  assert.doesNotMatch(source, />查看发布单<\/a-button>/, 'detail footer should not render 查看发布单')
  assert.doesNotMatch(source, /deleteManualArtifact\(selectedArtifact\)/, 'detail footer should not render a delete action')
  assert.doesNotMatch(
    source,
    /downloadArtifact\(selectedArtifact,\s*\$event\)/,
    'detail footer should not render the artifact download action',
  )
})

test('artifact center downloads through a hidden iframe instead of opening a new tab', () => {
  assert.match(
    source,
    /function downloadArtifact\(record: ReleaseOrderArtifactMetadataSummary,\s*event\?: MouseEvent\)/,
    'download handler should accept the click event so navigation can be suppressed',
  )
  assert.match(
    source,
    /event\?\.preventDefault\(\)[\s\S]*event\?\.stopPropagation\(\)/,
    'download handler should prevent default link-style navigation and bubbling',
  )
  assert.match(source, /document\.createElement\('iframe'\)/, 'download should create a hidden iframe')
  assert.match(source, /frame\.style\.display = 'none'/, 'download iframe should be hidden')
  assert.match(source, /frame\.src = url/, 'download iframe should load the artifact URL')
  assert.match(source, /document\.body\.appendChild\(frame\)/, 'download iframe should be appended to start download')
  assert.match(
    source,
    /window\.setTimeout\(\(\) => \{[\s\S]*frame\.remove\(\)[\s\S]*\}, 60_000\)/,
    'download iframe should be removed after the same timeout used by release detail',
  )
  assert.doesNotMatch(source, /window\.open\(/, 'artifact center should not open the artifact URL in a new tab')
  assert.match(
    source,
    /@click\.stop="downloadArtifact\(record, \$event\)"/,
    'grid download button should suppress parent tile interactions',
  )
})
