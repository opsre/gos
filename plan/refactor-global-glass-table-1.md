---
goal: Standardize Application Tables with the Light Frosted-Glass Visual System
version: 1.0
date_created: 2026-08-29
last_updated: 2026-08-29
owner: Codex
status: 'Completed'
tags: [refactor, frontend, table, visual-system, vue]
---

# Introduction

![Status: Completed](https://img.shields.io/badge/status-Completed-brightgreen)

Promote the light frosted-glass list treatment from the application workbench to page-level Ant Design tables, including release orders, while retaining each table's data, interaction, layout, and responsive behavior.

## 1. Requirements & Constraints

- **REQ-001**: Replace solid dark table headers inside `.app-layout .app-content` with translucent light headers using dark readable text.
- **REQ-002**: Render each table as one clipped frosted-glass surface with a light border, shared radius, subtle shadow, and row separators.
- **REQ-003**: Use light hover, selected, expanded, fixed-column, empty-state, and summary-row surfaces that remain visually continuous during horizontal scrolling.
- **REQ-004**: Apply the visual system to release order tables and all other page-level Ant Design tables without adding per-view duplicated CSS.
- **CON-001**: Do not change table data sources, columns, pagination, sorting, selection, expansion, permissions, routes, or backend APIs.
- **CON-002**: Scope the global table rules to `.app-layout .app-content` so login, sidebar, overlays teleported outside the content region, and unrelated components remain unchanged.
- **GUD-001**: Preserve semantic status colors and maintain readable text and keyboard focus against translucent backgrounds.
- **PAT-001**: Declare reusable table visual tokens in `frontend/src/style.css` and consume them through one centralized selector group.

## 2. Implementation Steps

### Implementation Phase 1

- GOAL-001: Define and apply the shared light glass table system.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-001 | Add reusable `--table-glass-*` tokens to `:root` in `frontend/src/style.css` for surface, header, row, hover, selected, border, radius, and shadow values. | ✅ | 2026-08-29 |
| TASK-002 | Replace the existing global dark `.table-card` header and zero-radius table rules in `frontend/src/style.css` with scoped `.app-layout .app-content .ant-table-wrapper` glass container, header, body, fixed-column, hover, selected, expanded, empty, and summary styles. | ✅ | 2026-08-29 |
| TASK-003 | Add shared light pagination and table control styling in `frontend/src/style.css` so sort, filter, and pagination controls remain coherent with the frosted table surface. | ✅ | 2026-08-29 |

### Implementation Phase 2

- GOAL-002: Verify broad compatibility and finalize the refactor.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-004 | Confirm the compiled CSS selector specificity overrides existing dark header declarations in release order, approval, project, template, notification, Jenkins, pipeline, executor, and artifact table views. | ✅ | 2026-08-29 |
| TASK-005 | Run `npm run build`, `npx vue-tsc --noEmit`, `git diff --check`, restart the Vite server on port 5174, and update this plan to `Completed`. | ✅ | 2026-08-29 |

## 3. Alternatives

- **ALT-001**: Edit every Vue view's scoped table CSS independently; rejected because it duplicates the visual system across more than a dozen files and will drift again.
- **ALT-002**: Replace all existing tables with a new wrapper component; rejected because it creates unnecessary markup and behavior migration risk for sorting, fixed columns, expanded rows, and virtual scrolling.

## 4. Dependencies

- **DEP-001**: Ant Design Vue table DOM classes provided by the existing frontend dependency.
- **DEP-002**: The `.app-layout` and `.app-content` shells rendered by `frontend/src/layouts/AppLayout.vue`.
- **DEP-003**: Existing global stylesheet import from `frontend/src/main.ts`.

## 5. Files

- **FILE-001**: `frontend/src/style.css` — define and apply the reusable table visual system.
- **FILE-002**: `plan/refactor-global-glass-table-1.md` — record scope, validation, and completion state.

## 6. Testing

- **TEST-001**: `npm run build` in `frontend/` exits successfully.
- **TEST-002**: `npx vue-tsc --noEmit` in `frontend/` exits successfully.
- **TEST-003**: `git diff --check` reports no whitespace errors.
- **TEST-004**: Compiled CSS contains the light shared header and glass container declarations with selectors more specific than existing view-scoped dark header rules.
- **TEST-005**: The Vite server on port 5174 serves the updated global stylesheet after restart.

## 7. Risks & Assumptions

- **RISK-001**: Fixed columns can reveal content behind translucent cells while horizontally scrolling; mitigate with a more opaque light fixed-cell token while retaining the glass palette.
- **RISK-002**: Existing view-scoped `!important` rules can override low-specificity theme rules; mitigate with a narrowly scoped, higher-specificity `.app-layout .app-content` selector group.
- **ASSUMPTION-001**: “Other page tables” refers to page-level Ant Design data tables in the authenticated application content area, including release-related views.

## 8. Related Specifications / Further Reading

[Global frontend styles](../frontend/src/style.css)
[Application list reference](../frontend/src/views/application/ApplicationListView.vue)
[Release order table](../frontend/src/views/release/ReleaseOrderListView.vue)
