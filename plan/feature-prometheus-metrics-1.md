---
goal: Implement one Prometheus endpoint for GOS release and application business metrics
version: 1
date_created: 2026-11-07
last_updated: 2026-11-07
owner: GOS
status: 'Completed'
tags: [feature, observability, prometheus]
---

# Introduction

![Status: Completed](https://img.shields.io/badge/status-Completed-brightgreen)

Implement the reviewed Grafana rendering's data contract through GET /metrics. Grafana UI deployment is outside this change.

## 1. Requirements & Constraints

- **REQ-001**: Export project/application metadata, application/environment state counts, created orders, terminal CI/CD results, duration histograms, latest release status, latest successful CD timestamp, approval backlog, and Agent state.
- **REQ-002**: Preserve counters and histogram observations across restarts, repeated tracker updates, and release deletion using transactionally inserted, uniquely keyed events.
- **REQ-003**: Poll configured HTTP JSON business sources and expose configured biz_ gauge/counter metrics with trusted application/environment/source labels.
- **SEC-001**: Make metrics opt-in; use a persisted administrator-controlled authentication switch. Protected access requires an independent bearer token; explicit anonymous access skips token checks. Non-local deployments without a saved policy default to protected access. Do not expose credentials, raw errors, release IDs, commit SHAs, or user IDs in labels.
- **CON-001**: Support SQLite and MySQL; serve immutable cached snapshots without database/network access during scrapes.
- **CON-002**: Export zero state/result series for known application/environment and execution combinations; retain last good data on collection failure and publish freshness/health.
- **PAT-001**: Share release business-state projection and Agent runtime-state rules with existing API behavior.

## 2. Implementation Steps

### Implementation Phase 1

- **GOAL-001**: Establish the persisted metrics contract and current-state repository. Complete when repository tests verify deduplication, rollback, deletion retention, and phase projection.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-001 | Add internal/domain/observability models/configuration and shared release/Agent state functions; delegate existing HTTP/usecase wrappers to the shared rules. | ✅ | 2026-11-07 |
| TASK-002 | Add the gos_metric_release_event schema/migration and transactional creation/completion recording in internal/infrastructure/persistence/sqlrepo/release_repository.go. Backfill existing records once. | ✅ | 2026-11-07 |
| TASK-003 | Implement MetricsRepository.LoadSnapshot in internal/infrastructure/persistence/sqlrepo/metrics_repository.go using aggregate queries and a consistent read transaction. Depends on TASK-001 and TASK-002. | ✅ | 2026-11-07 |

### Implementation Phase 2

- **GOAL-002**: Serve the complete metrics endpoint and document dashboard queries. Complete when focused/race tests and Go regression checks pass.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-004 | Implement internal/infrastructure/observability Exporter.Refresh, Run, Collect and bounded JSON-source collection; atomically publish cache snapshots. Depends on TASK-003. | ✅ | 2026-11-07 |
| TASK-005 | Add independent /metrics authentication, bootstrap configuration, server lifecycle wiring, Docker environment settings and explicit Nginx proxying. Depends on TASK-004. | ✅ | 2026-11-07 |
| TASK-006 | Add meaningful persistence/exporter/HTTP/config tests, docs/metrics.md, a Prometheus scrape example and Grafana query guidance; run go test ./... and focused race tests. Depends on TASK-005. | ✅ | 2026-11-07 |

### Implementation Phase 3

- **GOAL-003**: Add the metrics authentication switch to system settings with persistent, immediate runtime behavior. Complete when browser/API tests verify both modes and restart persistence.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-007 | Extend SystemManagementSettingsStore and DatabaseReleaseStore with nullable metrics_auth_enabled and transactional system-settings saves; publish the active policy through an atomic cache and synchronize other replicas every 5 seconds. | ✅ | 2026-11-07 |
| TASK-008 | Connect SystemSettingsHandler and the system-management Vue form to metrics_auth_enabled; make NewMetricsHandler consult the active cached policy and keep protected access closed when no token is configured. Depends on TASK-007. | ✅ | 2026-11-07 |
| TASK-009 | Verify disabled authentication gives anonymous 200, enabled authentication gives anonymous 401, missing protected token gives 503, rejected enabling without a token is atomic, browser saves work, and restart preserves the policy. Depends on TASK-008. | ✅ | 2026-11-07 |

## 3. Alternatives

- **ALT-001**: Counting current retained records as counters was rejected because editing/deletion would make counters decrease.
- **ALT-002**: Fetching dependencies during /metrics requests was rejected because slow sources would block scrapes and amplify load.

## 4. Dependencies

- **DEP-001**: github.com/prometheus/client_golang v1.23.2, with its official promhttp handler and Go/process collectors.
- **DEP-002**: Existing applications, projects, release, approval-flow and Agent tables, initialized by cmd/server/main.go.

## 5. Files

- **FILE-001**: internal/domain/observability/: configuration, aggregation types and duration buckets.
- **FILE-002**: internal/domain/release/business_status.go and internal/domain/agent/runtime_state.go: shared state rules.
- **FILE-003**: internal/infrastructure/persistence/sqlrepo/metrics*.go and release_repository.go: schema, transactional events and aggregates.
- **FILE-004**: internal/infrastructure/observability/: cached collector and HTTP JSON business sources.
- **FILE-005**: internal/interfaces/http/metrics_handler.go, internal/bootstrap/config.go and cmd/server/main.go: endpoint integration.
- **FILE-007**: frontend/src/views/system/SystemSettingsView.vue, frontend/src/types/system.ts, internal/application/usecase/system_management_settings.go and internal/infrastructure/configstore/release_db_store.go: persisted authentication switch and UI.
- **FILE-006**: docker/entrypoint.sh, docker/nginx.conf, configs/, .env.example, docs/metrics.md and docs/monitoring/: deployment and query examples.

## 6. Testing

- **TEST-001**: Persist one observation for repeated completion updates; rolled-back creates leave no event; deleting releases does not reduce counters/histograms; migration backfill is idempotent.
- **TEST-002**: Parse exported metrics and verify project/application/environment dimensions, all state zeros, histograms, offline Agent handling, cache retention and freshness after errors.
- **TEST-003**: Collect valid JSON gauges/counters; reject invalid/non-finite/missing values, oversize bodies, redirects and inconsistent metric configurations; isolate source failures.
- **TEST-004**: Verify independent bearer authentication, GET-only behavior, disabled 404, and closed access when authentication is enabled without a configured token.
- **TEST-006**: Browser toggle/save/reload verification and real SQLite settings-API tests passed, including production-mode anonymous access without a token and restart persistence. Frontend build passed; vue-tsc reports 112 identical pre-existing diagnostics against the HEAD baseline, with no new diagnostic lines.
- **TEST-005**: Run go test ./..., focused go test -race, go vet on affected packages and a local endpoint smoke test.

## 7. Risks & Assumptions

- **RISK-001**: Persistent events grow with release executions; retain them independently of release deletion. Future compaction must preserve counter and bucket totals.
- **RISK-002**: The endpoint contains database-global aggregates. Multi-replica installations must scrape one logical metrics exporter rather than sum duplicate replicas.
- **RISK-003**: Local Docker is not running; MySQL SQL was implemented but not exercised against a live MySQL server. SQLite endpoint integration, race tests and static analysis passed.
- **ASSUMPTION-001**: Business endpoints provide authoritative numeric values. Unconfigured business metrics remain absent rather than emitting demonstration values.
- **ASSUMPTION-002**: Historical project attribution follows the application's current project; application rename affects only metadata.

## 8. Related Specifications / Further Reading

- [Prometheus Go client](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus)
- [Instrumentation guidance](https://prometheus.io/docs/practices/instrumentation/)
- [GOS README](../README.md)
