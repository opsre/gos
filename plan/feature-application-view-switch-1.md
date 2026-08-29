---
goal: Add Card and List View Switching to the Application Workbench
version: 1.0
date_created: 2026-08-29
last_updated: 2026-08-29
owner: Codex
status: 'Completed'
tags: [feature, frontend, application-workbench, vue]
---

# Introduction

![Status: Completed](https://img.shields.io/badge/status-Completed-brightgreen)

Add a persistent card/list presentation switch to the application workbench without changing its API, filters, pagination, permissions, or application actions.

## 1. Requirements & Constraints

- **REQ-001**: Render a two-option control labeled `卡片` and `列表` in the application page toolbar.
- **REQ-002**: Use the existing `workbenchCards` computed value as the sole data source for both presentation modes.
- **REQ-003**: Preserve application status, latest release, activity status, release creation, release detail, configuration, edit, binding, template, and delete actions in list mode.
- **REQ-004**: Persist the selected presentation mode in `localStorage` and default to card mode when no valid stored value exists.
- **REQ-005**: Keep filtering, search, project selection, loading, empty state, and pagination behavior identical between modes.
- **REQ-006**: Present list mode as one light frosted-glass table surface with row separators, and keep the view switch transparent and light instead of using a dark active fill.
- **REQ-007**: Animate card/list content changes with a subtle non-overlapping transition while respecting the user's reduced-motion preference.
- **CON-001**: Do not change backend APIs, domain types, stores, or route definitions.
- **CON-002**: Keep the existing card presentation and release-detail interaction intact.
- **GUD-001**: Provide accessible labels, pressed state, keyboard-focus styling, and responsive layouts for the view switch and list rows.
- **PAT-001**: Reuse the existing action handlers and permission computed values in `frontend/src/views/application/ApplicationListView.vue`.

## 2. Implementation Steps

### Implementation Phase 1

- GOAL-001: Add deterministic view-mode state and toolbar controls.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-001 | Update `frontend/src/views/application/ApplicationListView.vue` imports and script state with `ApplicationViewMode`, `application-view-mode` storage handling, and a `setApplicationViewMode` function. | ✅ | 2026-08-29 |
| TASK-002 | Add the `卡片` and `列表` toolbar control with active, hover, focus, and `aria-pressed` states. | ✅ | 2026-08-29 |

### Implementation Phase 2

- GOAL-002: Implement the list presentation while preserving workbench behavior.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-003 | Gate the existing `application-workbench-columns` markup behind card mode and add list rows sourced from `workbenchCards`. | ✅ | 2026-08-29 |
| TASK-004 | Add list-row application metadata, statuses, latest release, activity, release actions, configuration popover, and inline release-detail content using existing handlers. | ✅ | 2026-08-29 |
| TASK-005 | Add responsive CSS that uses a column header on wide screens and stacked labeled cells below 1200px without horizontal overflow. | ✅ | 2026-08-29 |
| TASK-007 | Refine list mode into a unified light glass surface with separators and replace the dark switch active state with translucent white glass styling. | ✅ | 2026-08-29 |
| TASK-008 | Wrap card and list branches in an `out-in` Vue transition and add subtle opacity, vertical-offset, and scale states with a reduced-motion fallback. | ✅ | 2026-08-29 |

### Implementation Phase 3

- GOAL-003: Validate the feature and finalize documentation.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-006 | Run the Vue production build, validate formatting with `git diff --check`, and update this plan status and task completion fields. | ✅ | 2026-08-29 |

## 3. Alternatives

- **ALT-001**: Use `a-table`; rejected because the application workbench contains custom inline release detail, status animations, and permission-dependent actions that would require extensive slot duplication.
- **ALT-002**: Store the view mode in the route query; rejected because presentation mode is a local preference and must not alter shareable filter URLs or browser history.

## 4. Dependencies

- **DEP-001**: Vue 3 composition API already used by `ApplicationListView.vue`.
- **DEP-002**: Ant Design Vue icon and button components already installed in `frontend/package.json`.
- **DEP-003**: Existing `workbenchCards`, permission computed values, and navigation/action handlers in `ApplicationListView.vue`.

## 5. Files

- **FILE-001**: `frontend/src/views/application/ApplicationListView.vue` — add state, switch controls, list markup, and responsive styles.
- **FILE-002**: `plan/feature-application-view-switch-1.md` — record implementation scope and verification status.

## 6. Testing

- **TEST-001**: `npm run build` in `frontend/` exits successfully.
- **TEST-002**: Switching to list mode renders the same number of `workbenchCards` and retains filter and pagination state.
- **TEST-003**: Both modes expose permission-appropriate release, release-detail, and configuration actions.
- **TEST-004**: Refreshing the page restores a previously selected valid view mode and falls back to card mode for invalid storage values.
- **TEST-005**: List rows remain readable without horizontal page overflow at desktop, tablet, and mobile breakpoints.
- **TEST-006**: List rows have no per-row dark card surface, and neither view-switch option uses a solid dark active background.
- **TEST-007**: Switching between card and list mode animates only the workbench content, does not overlap both layouts, and disables motion under `prefers-reduced-motion: reduce`.

## 7. Risks & Assumptions

- **RISK-001**: Duplicated action markup can drift between card and list modes; mitigate by calling the same handlers and permission computed values in both branches.
- **RISK-002**: Dense list actions can overflow at intermediate widths; mitigate with a responsive grid and wrapped action region.
- **ASSUMPTION-001**: The requested list view is a compact alternative presentation of the existing application workbench, not a new server-side table endpoint.

## 8. Related Specifications / Further Reading

[Application workbench source](../frontend/src/views/application/ApplicationListView.vue)
[Frontend package configuration](../frontend/package.json)
