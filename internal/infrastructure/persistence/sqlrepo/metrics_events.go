package sqlrepo

import (
	"context"
	"database/sql"
	"fmt"

	release "gos/internal/domain/release"
)

const metricsMigrationVersion = "20261107_01_prometheus_release_events"

// Events are written in the release transaction and deliberately survive release
// deletion. Their unique keys make tracker retries and migration backfill idempotent.
func (r *ReleaseRepository) initMetricsSchema(ctx context.Context) error {
	statement := `CREATE TABLE IF NOT EXISTS gos_metric_release_event (
	event_id VARCHAR(160) PRIMARY KEY,
	kind VARCHAR(16) NOT NULL,
	application_id VARCHAR(64) NOT NULL,
	env_code VARCHAR(120) NOT NULL,
	trigger_type VARCHAR(50) NOT NULL,
	operation_type VARCHAR(32) NOT NULL,
	pipeline_scope VARCHAR(20) NOT NULL,
	provider VARCHAR(32) NOT NULL,
	result VARCHAR(32) NOT NULL,
	duration_seconds DOUBLE NULL,
	occurred_at BIGINT NOT NULL
)`
	if r.dbDriver == "mysql" {
		statement += ` ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	}
	if _, err := r.db.ExecContext(ctx, statement); err != nil {
		return err
	}
	if r.dbDriver == "mysql" {
		// Match the release table's collation so UNION scope queries also work on
		// databases upgraded from installations using utf8mb4_unicode_ci.
		var charset, collation string
		if err := r.db.QueryRowContext(ctx, `SELECT CHARACTER_SET_NAME, COLLATION_NAME FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'release_order' AND COLUMN_NAME = 'id'`).Scan(&charset, &collation); err != nil {
			return err
		}
		if !isSafeMySQLCharsetName(charset) || !isSafeMySQLCharsetName(collation) {
			return fmt.Errorf("invalid release metrics character set or collation")
		}
		if _, err := r.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE gos_metric_release_event CONVERT TO CHARACTER SET %s COLLATE %s", charset, collation)); err != nil {
			return err
		}
	}
	if err := r.ensureMetricsIndex(ctx); err != nil {
		return err
	}
	// No dependency on applications: release-schema initialization is also used
	// independently of application-schema initialization by migrations and tests.
	orderKey, executionKey := "'order:' || o.id", "'execution:' || e.id"
	if r.dbDriver == "mysql" {
		orderKey, executionKey = "CONCAT('order:', o.id)", "CONCAT('execution:', e.id)"
	}
	for _, query := range []string{
		metricInsertPrefix(r.dbDriver) + ` SELECT ` + orderKey + `, 'created', o.application_id, o.env_code, o.trigger_type,
		 o.operation_type, '', '', '', NULL, o.created_at FROM release_order o WHERE 1=1` + metricInsertSuffix(r.dbDriver),
		metricInsertPrefix(r.dbDriver) + ` SELECT ` + executionKey + `, 'execution', o.application_id, o.env_code, o.trigger_type,
		 o.operation_type, e.pipeline_scope, e.provider, e.status,
		 CASE WHEN e.status IN ('success','failed') AND e.started_at IS NOT NULL AND e.started_at > 0 AND e.finished_at >= e.started_at
		 THEN (e.finished_at - e.started_at) / 1000000000.0 ELSE NULL END, e.finished_at
		 FROM release_order_execution e JOIN release_order o ON o.id = e.release_order_id
		 WHERE e.status IN ('success','failed','cancelled','skipped') AND e.finished_at IS NOT NULL AND e.finished_at > 0` + metricInsertSuffix(r.dbDriver),
	} {
		if _, err := r.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("backfill release metrics: %w", err)
		}
	}
	return nil
}

func (r *ReleaseRepository) ensureMetricsIndex(ctx context.Context) error {
	for _, index := range []struct{ table, name, columns string }{
		{"gos_metric_release_event", "idx_metric_event_scope", "kind, application_id, env_code"},
		{"release_order", "idx_release_order_metric_latest", "application_id, env_code, created_at, id"},
	} {
		if r.dbDriver == "mysql" {
			var count int
			if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.STATISTICS
			 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?`, index.table, index.name).Scan(&count); err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			if _, err := r.db.ExecContext(ctx, fmt.Sprintf("CREATE INDEX %s ON %s (%s)", index.name, index.table, index.columns)); err != nil {
				return err
			}
		} else {
			if _, err := r.db.ExecContext(ctx, fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)", index.name, index.table, index.columns)); err != nil {
				return err
			}
		}
	}
	return nil
}

func metricInsertPrefix(driver string) string {
	return `INSERT INTO gos_metric_release_event
	(event_id, kind, application_id, env_code, trigger_type, operation_type, pipeline_scope, provider, result, duration_seconds, occurred_at)`
}

func metricInsertSuffix(driver string) string {
	if driver == "mysql" {
		return ` ON DUPLICATE KEY UPDATE event_id = event_id`
	}
	return ` ON CONFLICT(event_id) DO NOTHING`
}

func (r *ReleaseRepository) recordCreatedMetric(ctx context.Context, tx *sql.Tx, orderID string) error {
	_, err := tx.ExecContext(ctx, metricInsertPrefix(r.dbDriver)+`
	 SELECT ?, 'created', application_id, env_code, trigger_type, operation_type, '', '', '', NULL, created_at
	 FROM release_order WHERE id = ?`+metricInsertSuffix(r.dbDriver), "order:"+orderID, orderID)
	return err
}

func (r *ReleaseRepository) recordExecutionMetric(ctx context.Context, tx *sql.Tx, orderID string, scope release.PipelineScope) error {
	key := "'execution:' || e.id"
	if r.dbDriver == "mysql" {
		key = "CONCAT('execution:', e.id)"
	}
	_, err := tx.ExecContext(ctx, metricInsertPrefix(r.dbDriver)+` SELECT `+key+`, 'execution', o.application_id, o.env_code, o.trigger_type,
	 o.operation_type, e.pipeline_scope, e.provider, e.status,
	 CASE WHEN e.status IN ('success','failed') AND e.started_at IS NOT NULL AND e.started_at > 0 AND e.finished_at >= e.started_at
	 THEN (e.finished_at - e.started_at) / 1000000000.0 ELSE NULL END, e.finished_at
	 FROM release_order_execution e JOIN release_order o ON o.id = e.release_order_id
	 WHERE o.id = ? AND e.pipeline_scope = ? AND e.status IN ('success','failed','cancelled','skipped')
	 AND e.finished_at IS NOT NULL AND e.finished_at > 0`+metricInsertSuffix(r.dbDriver), orderID, string(scope))
	return err
}
