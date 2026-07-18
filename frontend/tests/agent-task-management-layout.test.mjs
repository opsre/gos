import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const sourceURL = new URL('../src/views/component/AgentTaskManagementView.vue', import.meta.url)
const source = readFileSync(sourceURL, 'utf8')

function extractStyleRule(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\n\\}`))
  assert.ok(match, `expected style rule for ${selector}`)
  return match[1]
}

test('agent task management uses the standard top bar without extra module wrappers', () => {
  assert.match(source, /<div class="page-wrap">[\s\S]*<div class="page-header">[\s\S]*<div class="page-title">任务<\/div>/, 'task page should use the standard page shell and title')
  assert.match(source, /<a-button class="application-toolbar-icon-btn" @click="openTaskSearchDialog">[\s\S]*<SearchOutlined \/>[\s\S]*<\/a-button>/, 'task search should use the same header icon button pattern as applications')
  assert.match(source, /class="application-toolbar-action-btn"[\s\S]*ReloadOutlined[\s\S]*刷新任务/, 'task refresh should move into the page header action strip')
  assert.match(source, /class="application-toolbar-action-btn"[\s\S]*PlusOutlined[\s\S]*新增任务/, 'task create action should use the standard header action button')
  assert.match(source, /<transition name="application-search-fade">[\s\S]*v-if="taskSearchDialogVisible"[\s\S]*class="application-search-overlay task-search-overlay"[\s\S]*<input[\s\S]*ref="taskSearchInputRef"[\s\S]*v-model="taskSearchDraft\.keyword"[\s\S]*class="application-search-floating-field"[\s\S]*placeholder="搜索任务 \/ Agent \/ 脚本"[\s\S]*@input="handleTaskSearchInput"[\s\S]*@keydown\.enter="handleTaskSearchSubmit"/, 'task search input should live in the application-style floating search overlay')
  assert.match(source, /v-for="item in visibleTaskSearchOptions"[\s\S]*class="application-search-suggestion"[\s\S]*@click="handleTaskSearchSelect\(item\)"/, 'task search overlay should render selectable task suggestions')
  assert.match(source, /const taskSearchOptions = computed[\s\S]*residentTaskList\.value\.map\(taskSearchOptionFromTask\)[\s\S]*historyTaskList\.value\.map\(taskSearchOptionFromTask\)/, 'global task search suggestions should include resident and temporary tasks')
  assert.match(source, /const visibleTaskSearchOptions = computed[\s\S]*taskSearchOptions\.value\.filter[\s\S]*filterTaskSearchOption/, 'floating task search should filter preselect options from the unified task list')
  assert.match(source, /function handleTaskSearchSelect\(option: TaskSearchOption\)[\s\S]*activeTaskTab\.value = option\.taskMode === 'resident' \? 'resident' : 'history'[\s\S]*closeTaskSearchDialog\(\)/, 'selecting a task search suggestion should switch to the matching task tab and close the overlay')
  assert.match(source, /function chooseTaskTabForKeyword[\s\S]*residentTaskList\.value\.some[\s\S]*historyTaskList\.value\.some[\s\S]*activeTaskTab\.value = 'resident'[\s\S]*activeTaskTab\.value = 'history'/, 'typing a search that only matches one task type should switch to that tab automatically')
  assert.match(source, /<\/a-modal>\s*<a-tabs v-model:activeKey="activeTaskTab" class="task-view-tabs"/, 'task tabs should sit directly on the page after modals and be controllable by search')
  assert.match(source, /const filteredResidentTaskList = computed[\s\S]*residentTaskList\.value\.filter[\s\S]*taskMatchesKeyword/, 'resident tasks should be filtered by the global task search')
  assert.match(source, /<a-tab-pane key="resident" tab="常驻任务">[\s\S]*filteredResidentTaskList/, 'resident task tab should render the filtered list')
  assert.match(source, /<a-tab-pane key="history" tab="临时任务">[\s\S]*pagedHistoryTaskList/, 'temporary task tab should be preserved')
  const historyPane = source.match(/<a-tab-pane key="history" tab="临时任务">([\s\S]*?)<\/a-tab-pane>/)?.[1] || ''
  assert.match(historyPane, /taskStatusColor\(item\.status\)[\s\S]*taskStatusText\(item\.status\)/, 'temporary task cards should show their current execution status')
  assert.match(source, /const AUTO_REFRESH_INTERVAL = 3000/, 'agent task state should refresh with a short near-realtime interval')
  assert.match(source, /autoRefreshInFlight[\s\S]*async function runAutoRefresh\(\)[\s\S]*loadAgents\(\{ silent: true \}\)[\s\S]*loadTaskViews\(\{ silent: true \}\)/, 'automatic refresh should avoid overlaps and reload agent plus task state')
  assert.match(source, /document\.addEventListener\('visibilitychange', handlePageVisibilityChange\)[\s\S]*window\.addEventListener\('focus', handleWindowFocus\)/, 'returning to the task page should trigger an immediate refresh')
  assert.doesNotMatch(source, /page-wrapper|page-header-card|filter-card|task-view-toolbar|任务视图|这里展示任务管理中维护的常驻任务模板|agent-task-module|agent-task-unified-layout|type="primary"|history-toolbar|history-toolbar-actions|component-toolbar-select|component-toolbar-query-btn|按 Agent 分类/, 'task page should not keep old shells, extra task-view card, local temporary filters, new module wrappers, or primary buttons')

  const tabsRule = extractStyleRule('.task-view-tabs')
  assert.match(tabsRule, /margin-top:\s*0/, 'task tabs should not add another section gap under the header')

  const tableCardRule = extractStyleRule('.table-card')
  assert.doesNotMatch(tableCardRule, /overflow:\s*hidden/, 'task list cards should not clip rounded content')

  const headerActionsRule = extractStyleRule('.page-header-actions')
  assert.match(headerActionsRule, /--task-header-action-bg:/, 'header actions should expose a shared surface background token')

  const iconButtonRule = extractStyleRule(':deep(.application-toolbar-icon-btn.ant-btn)')
  assert.match(iconButtonRule, /width:\s*42px/, 'task search icon button should match application page icon button dimensions')
  assert.match(iconButtonRule, /min-width:\s*42px/, 'task search icon button should keep a stable square footprint')
  assert.doesNotMatch(source, /task-global-search-shell|<a-auto-complete|class="task-global-search"/, 'task search should not render a persistent header input')
})

test('agent task create modal follows the standard button modal pattern', () => {
  const createModal = source.match(/<a-modal\s+v-model:open="createTaskVisible"([\s\S]*?)<\/a-modal>/)?.[0] || ''
  assert.ok(createModal, 'create task modal should exist')
  assert.match(createModal, /:width="760"/, 'create task modal should use the standard modal width')
  assert.match(createModal, /:closable="false"/, 'create task modal should remove the default close icon')
  assert.match(createModal, /:footer="null"/, 'create task modal should not use default footer buttons')
  assert.match(createModal, /:destroy-on-close="true"/, 'create task modal should destroy form content after close')
  assert.match(createModal, /:mask-style="modalMaskStyle"/, 'create task modal should use main-content-only mask styles')
  assert.match(createModal, /:wrap-props="modalWrapProps"/, 'create task modal should offset the wrapper from the sidebar')
  assert.match(createModal, /wrap-class-name="task-form-modal-wrap"/, 'create task modal should use a dedicated modal shell class')
  assert.match(createModal, /<template #title>[\s\S]*class="task-form-modal-titlebar"[\s\S]*class="task-form-modal-title"[\s\S]*新增任务[\s\S]*class="application-toolbar-action-btn task-form-modal-save-btn"[\s\S]*:loading="savingTask"[\s\S]*@click="handleCreateTask"[\s\S]*保存[\s\S]*<\/template>/, 'create task modal should put save in the title bar')
  assert.match(createModal, /<a-form layout="vertical" :required-mark="false" class="task-create-form">/, 'create task form should use vertical layout and hide default required marks')
  assert.match(createModal, /class="task-form-note"/, 'create task modal should use the standard lightweight note')
  assert.match(createModal, /class="task-form-panel"[\s\S]*class="task-form-panel-title"[\s\S]*任务配置/, 'create task modal should group fields in lightweight panels')
  assert.doesNotMatch(createModal, /title="新增任务"|:confirm-loading|ok-text|cancel-text|@ok=|task-target-card|selected-script-card|modal-subtitle/, 'create task modal should not keep default modal actions, title prop, or extra nested cards')

  assert.match(source, /const modalMaskStyle = computed[\s\S]*left: `\$\{modalViewportInset\.value\}px`[\s\S]*pointerEvents: createTaskVisible\.value \? 'auto' : 'none'/, 'create modal mask should only cover the main content area')
  assert.match(source, /const modalWrapProps = computed[\s\S]*width: `calc\(100% - \$\{modalViewportInset\.value\}px\)`[\s\S]*pointerEvents: createTaskVisible\.value \? 'auto' : 'none'/, 'create modal wrapper should release pointer events when closed')

  const modalContentRule = extractStyleRule('.task-form-modal-wrap :deep(.ant-modal-content)')
  assert.match(modalContentRule, /border-radius:\s*24px/, 'create modal shell should use the standard rounded surface')
  assert.match(modalContentRule, /overflow:\s*visible/, 'create modal shell should not clip the multi-select popup')
  assert.doesNotMatch(modalContentRule, /backdrop-filter/, 'interactive modal content should not create a blur stacking context')
  assert.match(createModal, /:get-popup-container="getTaskSelectPopupContainer"/, 'target Agent dropdown should mount inside the active modal')

  const panelRule = extractStyleRule('.task-form-panel')
  assert.doesNotMatch(panelRule, /box-shadow|background:\s*var\(--color-bg-card\)/, 'create modal panels should stay lightweight instead of nested heavy cards')
})
