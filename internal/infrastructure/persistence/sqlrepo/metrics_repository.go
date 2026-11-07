package sqlrepo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	agent "gos/internal/domain/agent"
	metrics "gos/internal/domain/observability"
	release "gos/internal/domain/release"
)

type MetricsRepository struct {
	db  *sql.DB
	now func() time.Time
}

func NewMetricsRepository(db *sql.DB) *MetricsRepository {
	return &MetricsRepository{db: db, now: time.Now}
}

// LoadSnapshot uses a single database snapshot and grouped queries. No credentials,
// logs or release IDs are loaded into the metrics representation.
func (r *MetricsRepository) LoadSnapshot(ctx context.Context) (metrics.Snapshot, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return metrics.Snapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var s metrics.Snapshot
	queries := []struct {
		query string
		scan  func(*sql.Rows) error
	}{
		{`SELECT id, app_key, name, project_id FROM applications`, func(rows *sql.Rows) error {
			var a metrics.Application
			if err := rows.Scan(&a.ID, &a.Key, &a.Name, &a.ProjectID); err != nil {
				return err
			}
			s.Applications = append(s.Applications, a)
			return nil
		}},
		{`SELECT id, project_key, name FROM projects`, func(rows *sql.Rows) error {
			var p metrics.Project
			if err := rows.Scan(&p.ID, &p.Key, &p.Name); err != nil {
				return err
			}
			s.Projects = append(s.Projects, p)
			return nil
		}},
		{`SELECT application_id, env_code FROM release_order
		 UNION SELECT application_id, env_code FROM gos_metric_release_event`, func(rows *sql.Rows) error {
			var v metrics.Scope
			if err := rows.Scan(&v.ApplicationID, &v.Env); err != nil {
				return err
			}
			s.Scopes = append(s.Scopes, v)
			return nil
		}},
		{`SELECT o.application_id, o.env_code, o.status, COALESCE(ci.status,''), COALESCE(cd.status,''), COUNT(*)
		 FROM release_order o
		 LEFT JOIN release_order_execution ci ON ci.release_order_id = o.id AND ci.pipeline_scope = 'ci'
		 LEFT JOIN release_order_execution cd ON cd.release_order_id = o.id AND cd.pipeline_scope = 'cd'
		 GROUP BY o.application_id, o.env_code, o.status, ci.status, cd.status`, func(rows *sql.Rows) error {
			var v metrics.OrderCount
			var status, ci, cd string
			if err := rows.Scan(&v.ApplicationID, &v.Env, &status, &ci, &cd, &v.Count); err != nil {
				return err
			}
			v.Status = projectMetricOrderStatus(status, ci, cd)
			s.Orders = append(s.Orders, v)
			return nil
		}},
		{`SELECT o.application_id, o.env_code, o.status, COALESCE(ci.status,''), COALESCE(cd.status,'')
		 FROM release_order o
		 LEFT JOIN release_order_execution ci ON ci.release_order_id = o.id AND ci.pipeline_scope = 'ci'
		 LEFT JOIN release_order_execution cd ON cd.release_order_id = o.id AND cd.pipeline_scope = 'cd'
		 WHERE NOT EXISTS (SELECT 1 FROM release_order newer WHERE newer.application_id = o.application_id
		 AND newer.env_code = o.env_code AND (newer.created_at > o.created_at OR (newer.created_at = o.created_at AND newer.id > o.id)))`, func(rows *sql.Rows) error {
			var v metrics.OrderCount
			var status, ci, cd string
			if err := rows.Scan(&v.ApplicationID, &v.Env, &status, &ci, &cd); err != nil {
				return err
			}
			v.Status, v.Count = projectMetricOrderStatus(status, ci, cd), 1
			s.LatestOrders = append(s.LatestOrders, v)
			return nil
		}},
		{`SELECT application_id, env_code, trigger_type, operation_type, COUNT(*)
		 FROM gos_metric_release_event WHERE kind = 'created'
		 GROUP BY application_id, env_code, trigger_type, operation_type`, func(rows *sql.Rows) error {
			var v metrics.CreatedCount
			if err := rows.Scan(&v.ApplicationID, &v.Env, &v.Trigger, &v.Operation, &v.Count); err != nil {
				return err
			}
			s.Created = append(s.Created, v)
			return nil
		}},
		{`SELECT application_id, env_code, pipeline_scope, provider, result, COUNT(*)
		 FROM gos_metric_release_event WHERE kind = 'execution'
		 GROUP BY application_id, env_code, pipeline_scope, provider, result`, func(rows *sql.Rows) error {
			var v metrics.ExecutionCount
			if err := rows.Scan(&v.ApplicationID, &v.Env, &v.PipelineScope, &v.Provider, &v.Result, &v.Count); err != nil {
				return err
			}
			s.Executions = append(s.Executions, v)
			return nil
		}},
		{`SELECT o.application_id, o.env_code, e.pipeline_scope, e.provider
		 FROM release_order_execution e JOIN release_order o ON o.id = e.release_order_id
		 GROUP BY o.application_id, o.env_code, e.pipeline_scope, e.provider
		 UNION SELECT application_id, env_code, pipeline_scope, provider FROM gos_metric_release_event WHERE kind = 'execution'`, func(rows *sql.Rows) error {
			var v metrics.ExecutionScope
			if err := rows.Scan(&v.ApplicationID, &v.Env, &v.PipelineScope, &v.Provider); err != nil {
				return err
			}
			s.ExecutionScopes = append(s.ExecutionScopes, v)
			return nil
		}},
		{`SELECT application_id, env_code, MAX(occurred_at) / 1000000000.0
		 FROM gos_metric_release_event WHERE kind = 'execution' AND pipeline_scope = 'cd' AND result = 'success'
		 GROUP BY application_id, env_code`, func(rows *sql.Rows) error {
			var v metrics.Timestamp
			if err := rows.Scan(&v.ApplicationID, &v.Env, &v.Seconds); err != nil {
				return err
			}
			s.LastSuccess = append(s.LastSuccess, v)
			return nil
		}},
		{`SELECT o.application_id, o.env_code, t.gate, COUNT(*) FROM release_order_approval_flow_task t
		 JOIN release_order o ON o.id = t.release_order_id
		 WHERE t.node_type = 'approval' AND t.status = 'pending'
		 AND o.status NOT IN ('cancelled','rejected','success','deploy_success','failed','deploy_failed')
		 GROUP BY o.application_id, o.env_code, t.gate
		 UNION ALL SELECT o.application_id, o.env_code, 'before_execute', COUNT(*) FROM release_order o
		 WHERE o.approval_required = 1 AND o.status IN ('pending_approval','approving')
		 AND NOT EXISTS (SELECT 1 FROM release_order_approval_flow_instance i WHERE i.release_order_id = o.id)
		 GROUP BY o.application_id, o.env_code`, func(rows *sql.Rows) error {
			var v metrics.ApprovalCount
			if err := rows.Scan(&v.ApplicationID, &v.Env, &v.Gate, &v.Count); err != nil {
				return err
			}
			s.Approvals = append(s.Approvals, v)
			return nil
		}},
	}
	for i, q := range queries {
		if err := scanMetricRows(ctx, tx, q.query, nil, q.scan); err != nil {
			return metrics.Snapshot{}, fmt.Errorf("metrics snapshot query %d: %w", i+1, err)
		}
	}
	if err := loadMetricDurations(ctx, tx, &s); err != nil {
		return metrics.Snapshot{}, err
	}
	now := r.now()
	if err := scanMetricRows(ctx, tx, `SELECT id, agent_code, name, environment_code, status, last_heartbeat_at, current_task_id FROM agent_instance`, nil, func(rows *sql.Rows) error {
		var instance agent.Instance
		var heartbeat int64
		if err := rows.Scan(&instance.ID, &instance.AgentCode, &instance.Name, &instance.EnvironmentCode, &instance.Status, &heartbeat, &instance.CurrentTaskID); err != nil {
			return err
		}
		if heartbeat > 0 {
			instance.LastHeartbeatAt = time.Unix(0, heartbeat)
		}
		s.Agents = append(s.Agents, metrics.Agent{
			ID: instance.ID, Code: instance.AgentCode, Name: instance.Name, Env: instance.EnvironmentCode,
			Status: string(agent.ResolveRuntimeState(instance, now, agent.DefaultOfflineAfter)), LastHeartbeat: float64(heartbeat) / 1e9,
		})
		return nil
	}); err != nil {
		return metrics.Snapshot{}, fmt.Errorf("metrics agents: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return metrics.Snapshot{}, err
	}
	return s, nil
}

func scanMetricRows(ctx context.Context, tx *sql.Tx, query string, args []any, scan func(*sql.Rows) error) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func projectMetricOrderStatus(status, ci, cd string) string {
	var executions []release.ReleaseOrderExecution
	for _, item := range []struct {
		scope  release.PipelineScope
		status string
	}{{release.PipelineScopeCI, ci}, {release.PipelineScopeCD, cd}} {
		if item.status != "" {
			executions = append(executions, release.ReleaseOrderExecution{PipelineScope: item.scope, Status: release.ExecutionStatus(item.status)})
		}
	}
	return string(release.DeriveBusinessStatus(release.OrderStatus(status), executions))
}

func loadMetricDurations(ctx context.Context, tx *sql.Tx, s *metrics.Snapshot) error {
	var columns []string
	var args []any
	for _, upper := range metrics.DurationBuckets {
		columns = append(columns, `SUM(CASE WHEN duration_seconds <= ? THEN 1 ELSE 0 END)`)
		args = append(args, upper)
	}
	query := `SELECT application_id, env_code, pipeline_scope, provider, COUNT(*), SUM(duration_seconds), ` + strings.Join(columns, ",") + `
	 FROM gos_metric_release_event WHERE kind = 'execution' AND duration_seconds IS NOT NULL
	 GROUP BY application_id, env_code, pipeline_scope, provider`
	return scanMetricRows(ctx, tx, query, args, func(rows *sql.Rows) error {
		var v metrics.Duration
		counts := make([]uint64, len(metrics.DurationBuckets))
		dest := []any{&v.ApplicationID, &v.Env, &v.PipelineScope, &v.Provider, &v.Count, &v.Sum}
		for i := range counts {
			dest = append(dest, &counts[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		v.Buckets = make(map[float64]uint64, len(counts))
		for i, upper := range metrics.DurationBuckets {
			v.Buckets[upper] = counts[i]
		}
		s.Durations = append(s.Durations, v)
		return nil
	})
}
