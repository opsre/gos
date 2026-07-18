package sqlrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/release"
)

func (r *ReleaseRepository) initApprovalFlowSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS release_approval_flow_definition (
			id VARCHAR(64) PRIMARY KEY,
			name VARCHAR(128) NOT NULL,
			status VARCHAR(32) NOT NULL,
			nodes_json TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS release_order_approval_flow_instance (
			id VARCHAR(64) PRIMARY KEY,
			release_order_id VARCHAR(64) NOT NULL UNIQUE,
			flow_definition_id VARCHAR(64) NOT NULL,
			flow_name VARCHAR(128) NOT NULL,
			flow_snapshot_json TEXT NOT NULL,
			status VARCHAR(32) NOT NULL,
			current_gate VARCHAR(32) NOT NULL DEFAULT '',
			current_scope VARCHAR(32) NOT NULL DEFAULT '',
			current_node_code VARCHAR(64) NOT NULL DEFAULT '',
			current_task_id VARCHAR(64) NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS release_order_approval_flow_task (
			id VARCHAR(64) PRIMARY KEY,
			instance_id VARCHAR(64) NOT NULL,
			release_order_id VARCHAR(64) NOT NULL,
			node_code VARCHAR(64) NOT NULL,
			node_name VARCHAR(128) NOT NULL,
			gate VARCHAR(32) NOT NULL,
			node_type VARCHAR(32) NOT NULL DEFAULT 'approval',
			approval_mode VARCHAR(32) NOT NULL,
			approver_ids_json TEXT NOT NULL,
			approver_names_json TEXT NOT NULL,
			agent_task_id VARCHAR(64) NOT NULL DEFAULT '',
			agent_task_name VARCHAR(128) NOT NULL DEFAULT '',
			agent_batch_id VARCHAR(64) NOT NULL DEFAULT '',
			message VARCHAR(2000) NOT NULL DEFAULT '',
			status VARCHAR(32) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS release_order_approval_flow_task_record (
			id VARCHAR(64) PRIMARY KEY,
			task_id VARCHAR(64) NOT NULL,
			action VARCHAR(32) NOT NULL,
			operator_user_id VARCHAR(64) NOT NULL DEFAULT '',
			operator_name VARCHAR(128) NOT NULL DEFAULT '',
			comment VARCHAR(1000) NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS release_application_approval_flow_binding (
			application_id VARCHAR(64) PRIMARY KEY,
			approval_flow_id VARCHAR(64) NOT NULL DEFAULT '',
			updated_at BIGINT NOT NULL
		);`,
	}
	for _, stmt := range statements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := r.ensureApprovalFlowInstanceColumns(ctx); err != nil {
		return err
	}
	return r.ensureApprovalFlowTaskColumns(ctx)
}

func (r *ReleaseRepository) ensureApprovalFlowInstanceColumns(ctx context.Context) error {
	columns := []struct {
		name      string
		mysqlDDL  string
		sqliteDDL string
	}{
		{"current_scope", `ALTER TABLE release_order_approval_flow_instance ADD COLUMN current_scope VARCHAR(32) NOT NULL DEFAULT '' AFTER current_gate;`, `ALTER TABLE release_order_approval_flow_instance ADD COLUMN current_scope TEXT NOT NULL DEFAULT '';`},
		{"current_node_code", `ALTER TABLE release_order_approval_flow_instance ADD COLUMN current_node_code VARCHAR(64) NOT NULL DEFAULT '' AFTER current_scope;`, `ALTER TABLE release_order_approval_flow_instance ADD COLUMN current_node_code TEXT NOT NULL DEFAULT '';`},
	}
	for _, column := range columns {
		var exists bool
		var err error
		switch r.dbDriver {
		case "mysql":
			exists, err = r.mysqlColumnExists(ctx, "release_order_approval_flow_instance", column.name)
		case "sqlite":
			var tableColumns map[string]struct{}
			tableColumns, err = r.sqliteTableColumns(ctx, "release_order_approval_flow_instance")
			_, exists = tableColumns[column.name]
		default:
			return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
		}
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		ddl := column.sqliteDDL
		if r.dbDriver == "mysql" {
			ddl = column.mysqlDDL
		}
		if _, err := r.db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReleaseRepository) ensureApprovalFlowTaskColumns(ctx context.Context) error {
	columns := []struct {
		name      string
		mysqlDDL  string
		sqliteDDL string
	}{
		{"node_type", `ALTER TABLE release_order_approval_flow_task ADD COLUMN node_type VARCHAR(32) NOT NULL DEFAULT 'approval' AFTER gate;`, `ALTER TABLE release_order_approval_flow_task ADD COLUMN node_type TEXT NOT NULL DEFAULT 'approval';`},
		{"agent_task_id", `ALTER TABLE release_order_approval_flow_task ADD COLUMN agent_task_id VARCHAR(64) NOT NULL DEFAULT '' AFTER approver_names_json;`, `ALTER TABLE release_order_approval_flow_task ADD COLUMN agent_task_id TEXT NOT NULL DEFAULT '';`},
		{"agent_task_name", `ALTER TABLE release_order_approval_flow_task ADD COLUMN agent_task_name VARCHAR(128) NOT NULL DEFAULT '' AFTER agent_task_id;`, `ALTER TABLE release_order_approval_flow_task ADD COLUMN agent_task_name TEXT NOT NULL DEFAULT '';`},
		{"agent_batch_id", `ALTER TABLE release_order_approval_flow_task ADD COLUMN agent_batch_id VARCHAR(64) NOT NULL DEFAULT '' AFTER agent_task_name;`, `ALTER TABLE release_order_approval_flow_task ADD COLUMN agent_batch_id TEXT NOT NULL DEFAULT '';`},
		{"message", `ALTER TABLE release_order_approval_flow_task ADD COLUMN message VARCHAR(2000) NOT NULL DEFAULT '' AFTER agent_batch_id;`, `ALTER TABLE release_order_approval_flow_task ADD COLUMN message TEXT NOT NULL DEFAULT '';`},
	}
	for _, column := range columns {
		var exists bool
		var err error
		switch r.dbDriver {
		case "mysql":
			exists, err = r.mysqlColumnExists(ctx, "release_order_approval_flow_task", column.name)
		case "sqlite":
			var tableColumns map[string]struct{}
			tableColumns, err = r.sqliteTableColumns(ctx, "release_order_approval_flow_task")
			_, exists = tableColumns[column.name]
		default:
			return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
		}
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		ddl := column.sqliteDDL
		if r.dbDriver == "mysql" {
			ddl = column.mysqlDDL
		}
		if _, err := r.db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReleaseRepository) GetApplicationApprovalFlowID(ctx context.Context, applicationID string) (string, error) {
	flowID, _, err := r.GetApplicationApprovalFlowBinding(ctx, applicationID)
	return flowID, err
}

func (r *ReleaseRepository) GetApplicationApprovalFlowBinding(ctx context.Context, applicationID string) (string, bool, error) {
	var flowID string
	err := r.db.QueryRowContext(ctx, `SELECT approval_flow_id FROM release_application_approval_flow_binding WHERE application_id = ?;`, strings.TrimSpace(applicationID)).Scan(&flowID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(flowID), true, nil
}

func (r *ReleaseRepository) UpsertApplicationApprovalFlowID(ctx context.Context, applicationID string, approvalFlowID string, updatedAt time.Time) error {
	applicationID, approvalFlowID = strings.TrimSpace(applicationID), strings.TrimSpace(approvalFlowID)
	if r.dbDriver == "mysql" {
		_, err := r.db.ExecContext(ctx, `INSERT INTO release_application_approval_flow_binding (application_id, approval_flow_id, updated_at) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE approval_flow_id = VALUES(approval_flow_id), updated_at = VALUES(updated_at);`, applicationID, approvalFlowID, updatedAt.UTC().UnixNano())
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO release_application_approval_flow_binding (application_id, approval_flow_id, updated_at) VALUES (?, ?, ?) ON CONFLICT(application_id) DO UPDATE SET approval_flow_id = excluded.approval_flow_id, updated_at = excluded.updated_at;`, applicationID, approvalFlowID, updatedAt.UTC().UnixNano())
	return err
}

func (r *ReleaseRepository) CreateApprovalFlowDefinition(ctx context.Context, item domain.ApprovalFlowDefinition) error {
	nodes, err := marshalApprovalFlowDefinition(item.Nodes, item.Links)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO release_approval_flow_definition (id, name, status, nodes_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);`, item.ID, strings.TrimSpace(item.Name), string(item.Status), nodes, item.CreatedAt.UTC().UnixNano(), item.UpdatedAt.UTC().UnixNano())
	return err
}

func (r *ReleaseRepository) UpdateApprovalFlowDefinition(ctx context.Context, item domain.ApprovalFlowDefinition) error {
	nodes, err := marshalApprovalFlowDefinition(item.Nodes, item.Links)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE release_approval_flow_definition
SET name = ?, status = ?, nodes_json = ?, updated_at = ?
WHERE id = ?;`, strings.TrimSpace(item.Name), string(item.Status), nodes, item.UpdatedAt.UTC().UnixNano(), item.ID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return domain.ErrApprovalFlowNotFound
	}
	return nil
}

func (r *ReleaseRepository) GetApprovalFlowDefinitionByID(ctx context.Context, id string) (domain.ApprovalFlowDefinition, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, name, status, nodes_json, created_at, updated_at
FROM release_approval_flow_definition WHERE id = ?;`, strings.TrimSpace(id))
	item, err := scanApprovalFlowDefinition(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ApprovalFlowDefinition{}, domain.ErrApprovalFlowNotFound
	}
	return item, err
}

func (r *ReleaseRepository) ListApprovalFlowDefinitions(ctx context.Context, status domain.ApprovalFlowStatus) ([]domain.ApprovalFlowDefinition, error) {
	query := `SELECT id, name, status, nodes_json, created_at, updated_at FROM release_approval_flow_definition`
	args := []any{}
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, string(status))
	}
	query += " ORDER BY updated_at DESC, id DESC;"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.ApprovalFlowDefinition, 0)
	for rows.Next() {
		item, scanErr := scanApprovalFlowDefinition(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ReleaseRepository) CreateApprovalFlowInstance(ctx context.Context, item domain.ReleaseOrderApprovalFlowInstance) error {
	nodes, err := marshalApprovalFlowDefinition(item.Nodes, item.Links)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO release_order_approval_flow_instance (
 id, release_order_id, flow_definition_id, flow_name, flow_snapshot_json, status, current_gate, current_scope, current_node_code, current_task_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		item.ID, item.ReleaseOrderID, item.FlowDefinitionID, item.FlowName, nodes, string(item.Status), string(item.CurrentGate), string(item.CurrentScope), item.CurrentNodeCode, item.CurrentTaskID,
		item.CreatedAt.UTC().UnixNano(), item.UpdatedAt.UTC().UnixNano())
	return err
}

func (r *ReleaseRepository) GetApprovalFlowInstanceByOrderID(ctx context.Context, releaseOrderID string) (domain.ReleaseOrderApprovalFlowInstance, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, release_order_id, flow_definition_id, flow_name, flow_snapshot_json, status, current_gate, current_scope, current_node_code, current_task_id, created_at, updated_at
FROM release_order_approval_flow_instance WHERE release_order_id = ?;`, strings.TrimSpace(releaseOrderID))
	item, err := scanApprovalFlowInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ReleaseOrderApprovalFlowInstance{}, domain.ErrApprovalFlowInstanceNotFound
	}
	return item, err
}

func (r *ReleaseRepository) UpdateApprovalFlowInstance(ctx context.Context, item domain.ReleaseOrderApprovalFlowInstance) error {
	snapshot, err := marshalApprovalFlowDefinition(item.Nodes, item.Links)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE release_order_approval_flow_instance
SET flow_definition_id = ?, flow_name = ?, flow_snapshot_json = ?, status = ?, current_gate = ?, current_scope = ?, current_node_code = ?, current_task_id = ?, updated_at = ?
WHERE id = ?;`, item.FlowDefinitionID, item.FlowName, snapshot, string(item.Status), string(item.CurrentGate), string(item.CurrentScope), item.CurrentNodeCode, item.CurrentTaskID, item.UpdatedAt.UTC().UnixNano(), item.ID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return domain.ErrApprovalFlowInstanceNotFound
	}
	return nil
}

func (r *ReleaseRepository) DeleteApprovalFlowInstance(ctx context.Context, releaseOrderID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM release_order_approval_flow_instance WHERE release_order_id = ?;`, strings.TrimSpace(releaseOrderID))
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return domain.ErrApprovalFlowInstanceNotFound
	}
	return nil
}

func (r *ReleaseRepository) CreateApprovalFlowTask(ctx context.Context, item domain.ReleaseOrderApprovalFlowTask) error {
	ids, err := json.Marshal(item.ApproverIDs)
	if err != nil {
		return err
	}
	names, err := json.Marshal(item.ApproverNames)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO release_order_approval_flow_task (
 id, instance_id, release_order_id, node_code, node_name, gate, node_type, approval_mode, approver_ids_json, approver_names_json,
 agent_task_id, agent_task_name, agent_batch_id, message, status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		item.ID, item.InstanceID, item.ReleaseOrderID, item.NodeCode, item.NodeName, string(item.Gate), string(item.NodeType), string(item.ApprovalMode), string(ids), string(names),
		item.AgentTaskID, item.AgentTaskName, item.AgentBatchID, item.Message, string(item.Status), item.CreatedAt.UTC().UnixNano(), item.UpdatedAt.UTC().UnixNano())
	return err
}

func (r *ReleaseRepository) GetApprovalFlowTaskByID(ctx context.Context, id string) (domain.ReleaseOrderApprovalFlowTask, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, instance_id, release_order_id, node_code, node_name, gate, node_type, approval_mode, approver_ids_json, approver_names_json,
       agent_task_id, agent_task_name, agent_batch_id, message, status, created_at, updated_at
FROM release_order_approval_flow_task WHERE id = ?;`, strings.TrimSpace(id))
	item, err := scanApprovalFlowTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ReleaseOrderApprovalFlowTask{}, domain.ErrApprovalFlowTaskNotFound
	}
	return item, err
}

func (r *ReleaseRepository) ListApprovalFlowTasks(ctx context.Context, releaseOrderID string) ([]domain.ReleaseOrderApprovalFlowTask, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, instance_id, release_order_id, node_code, node_name, gate, node_type, approval_mode, approver_ids_json, approver_names_json,
       agent_task_id, agent_task_name, agent_batch_id, message, status, created_at, updated_at
FROM release_order_approval_flow_task WHERE release_order_id = ? ORDER BY created_at ASC, id ASC;`, strings.TrimSpace(releaseOrderID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.ReleaseOrderApprovalFlowTask, 0)
	for rows.Next() {
		item, scanErr := scanApprovalFlowTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ReleaseRepository) UpdateApprovalFlowTask(ctx context.Context, item domain.ReleaseOrderApprovalFlowTask) error {
	result, err := r.db.ExecContext(ctx, `UPDATE release_order_approval_flow_task SET status = ?, message = ?, updated_at = ? WHERE id = ?;`, string(item.Status), item.Message, item.UpdatedAt.UTC().UnixNano(), item.ID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return domain.ErrApprovalFlowTaskNotFound
	}
	return nil
}

func (r *ReleaseRepository) CreateApprovalFlowTaskRecord(ctx context.Context, item domain.ReleaseOrderApprovalFlowTaskRecord) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO release_order_approval_flow_task_record (id, task_id, action, operator_user_id, operator_name, comment, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);`, item.ID, item.TaskID, string(item.Action), item.OperatorUserID, item.OperatorName, item.Comment, item.CreatedAt.UTC().UnixNano())
	return err
}

func (r *ReleaseRepository) ListApprovalFlowTaskRecords(ctx context.Context, taskID string) ([]domain.ReleaseOrderApprovalFlowTaskRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, task_id, action, operator_user_id, operator_name, comment, created_at FROM release_order_approval_flow_task_record WHERE task_id = ? ORDER BY created_at ASC, id ASC;`, strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.ReleaseOrderApprovalFlowTaskRecord, 0)
	for rows.Next() {
		var item domain.ReleaseOrderApprovalFlowTaskRecord
		var createdAt int64
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Action, &item.OperatorUserID, &item.OperatorName, &item.Comment, &createdAt); err != nil {
			return nil, err
		}
		item.CreatedAt = time.Unix(0, createdAt).UTC()
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListApprovalWorkbenchTasks 聚合应用级审批流任务和历史模板审批待办。
func (r *ReleaseRepository) ListApprovalWorkbenchTasks(ctx context.Context, filter domain.ApprovalWorkbenchListFilter) ([]domain.ReleaseApprovalWorkbenchTask, int64, error) {
	filter = normalizeApprovalWorkbenchListFilter(filter)
	userID := strings.TrimSpace(filter.UserID)
	if userID == "" {
		return []domain.ReleaseApprovalWorkbenchTask{}, 0, nil
	}
	pattern := "%\"" + userID + "\"%"
	unionQuery := `
SELECT
  'flow' AS source,
  t.id AS task_id,
  ro.id AS release_order_id,
  ro.order_no,
  ro.application_id,
  ro.application_name,
  ro.env_code,
  ro.operation_type,
  ro.triggered_by,
  i.flow_name,
  t.node_name,
  t.gate,
  i.current_scope AS execution_scope,
  t.approval_mode,
  t.approver_ids_json,
  t.approver_names_json,
  ro.status AS release_order_status,
  t.created_at,
  t.updated_at
FROM release_order_approval_flow_task t
INNER JOIN release_order_approval_flow_instance i ON i.id = t.instance_id AND i.current_task_id = t.id
INNER JOIN release_order ro ON ro.id = t.release_order_id
WHERE t.status = 'pending'
  AND t.approver_ids_json LIKE ?
  AND NOT EXISTS (
    SELECT 1
    FROM release_order_approval_flow_task_record acted
    WHERE acted.task_id = t.id AND acted.operator_user_id = ?
  )
UNION ALL
SELECT
  'legacy' AS source,
  '' AS task_id,
  ro.id AS release_order_id,
  ro.order_no,
  ro.application_id,
  ro.application_name,
  ro.env_code,
  ro.operation_type,
  ro.triggered_by,
  '历史审批' AS flow_name,
  '发布审批' AS node_name,
  'before_execute' AS gate,
  '' AS execution_scope,
  ro.approval_mode,
  ro.approval_approver_ids_json,
  ro.approval_approver_names_json,
  ro.status AS release_order_status,
  ro.created_at,
  ro.updated_at
FROM release_order ro
WHERE ro.approval_required = 1
  AND ro.status IN ('pending_approval', 'approving')
  AND ro.approval_approver_ids_json LIKE ?
  AND NOT EXISTS (
    SELECT 1
    FROM release_order_approval_flow_instance existing_flow
    WHERE existing_flow.release_order_id = ro.id
  )
  AND NOT EXISTS (
    SELECT 1
    FROM release_order_approval_record acted
    WHERE acted.release_order_id = ro.id
      AND acted.operator_user_id = ?
      AND acted.action IN ('approve', 'reject')
  )`
	args := []any{pattern, userID, pattern, userID}
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM ("+unionQuery+") approval_workbench_tasks", args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	rows, err := r.db.QueryContext(ctx, "SELECT * FROM ("+unionQuery+") approval_workbench_tasks ORDER BY created_at DESC, release_order_id DESC LIMIT ? OFFSET ?", append(args, filter.PageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]domain.ReleaseApprovalWorkbenchTask, 0)
	for rows.Next() {
		item, scanErr := scanReleaseApprovalWorkbenchTask(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// ListApprovalWorkbenchRecords 聚合当前用户处理过的新旧审批记录。
func (r *ReleaseRepository) ListApprovalWorkbenchRecords(ctx context.Context, filter domain.ApprovalWorkbenchListFilter) ([]domain.ReleaseApprovalWorkbenchRecord, int64, error) {
	filter = normalizeApprovalWorkbenchListFilter(filter)
	userID := strings.TrimSpace(filter.UserID)
	if userID == "" {
		return []domain.ReleaseApprovalWorkbenchRecord{}, 0, nil
	}
	unionQuery := `
SELECT
  tr.id,
  'flow' AS source,
  t.id AS task_id,
  ro.id AS release_order_id,
  ro.order_no,
  ro.application_id,
  ro.application_name,
  ro.env_code,
  ro.operation_type,
  ro.triggered_by,
  i.flow_name,
  t.node_name,
  t.gate,
  i.current_scope AS execution_scope,
  tr.action,
  tr.operator_user_id,
  tr.operator_name,
  tr.comment,
  ro.status AS release_order_status,
  tr.created_at
FROM release_order_approval_flow_task_record tr
INNER JOIN release_order_approval_flow_task t ON t.id = tr.task_id
INNER JOIN release_order_approval_flow_instance i ON i.id = t.instance_id
INNER JOIN release_order ro ON ro.id = t.release_order_id
WHERE tr.operator_user_id = ? AND tr.action IN ('approve', 'reject')
UNION ALL
SELECT
  ar.id,
  'legacy' AS source,
  '' AS task_id,
  ro.id AS release_order_id,
  ro.order_no,
  ro.application_id,
  ro.application_name,
  ro.env_code,
  ro.operation_type,
  ro.triggered_by,
  '历史审批' AS flow_name,
  '发布审批' AS node_name,
  'before_execute' AS gate,
  '' AS execution_scope,
  ar.action,
  ar.operator_user_id,
  ar.operator_name,
  ar.comment,
  ro.status AS release_order_status,
  ar.created_at
FROM release_order_approval_record ar
INNER JOIN release_order ro ON ro.id = ar.release_order_id
WHERE ar.operator_user_id = ? AND ar.action IN ('approve', 'reject')`
	args := []any{userID, userID}
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM ("+unionQuery+") approval_workbench_records", args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	rows, err := r.db.QueryContext(ctx, "SELECT * FROM ("+unionQuery+") approval_workbench_records ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?", append(args, filter.PageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]domain.ReleaseApprovalWorkbenchRecord, 0)
	for rows.Next() {
		item, scanErr := scanReleaseApprovalWorkbenchRecord(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func normalizeApprovalWorkbenchListFilter(filter domain.ApprovalWorkbenchListFilter) domain.ApprovalWorkbenchListFilter {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	return filter
}

func scanReleaseApprovalWorkbenchTask(scanner interface{ Scan(...any) error }) (domain.ReleaseApprovalWorkbenchTask, error) {
	var item domain.ReleaseApprovalWorkbenchTask
	var approverIDsJSON, approverNamesJSON string
	var createdAt, updatedAt int64
	if err := scanner.Scan(
		&item.Source, &item.TaskID, &item.ReleaseOrderID, &item.OrderNo, &item.ApplicationID, &item.ApplicationName,
		&item.EnvCode, &item.OperationType, &item.TriggeredBy, &item.FlowName, &item.NodeName, &item.Gate,
		&item.ExecutionScope, &item.ApprovalMode, &approverIDsJSON, &approverNamesJSON, &item.ReleaseOrderStatus,
		&createdAt, &updatedAt,
	); err != nil {
		return item, err
	}
	if err := json.Unmarshal([]byte(approverIDsJSON), &item.ApproverIDs); err != nil {
		return item, err
	}
	if err := json.Unmarshal([]byte(approverNamesJSON), &item.ApproverNames); err != nil {
		return item, err
	}
	item.CreatedAt, item.UpdatedAt = time.Unix(0, createdAt).UTC(), time.Unix(0, updatedAt).UTC()
	return item, nil
}

func scanReleaseApprovalWorkbenchRecord(scanner interface{ Scan(...any) error }) (domain.ReleaseApprovalWorkbenchRecord, error) {
	var item domain.ReleaseApprovalWorkbenchRecord
	var createdAt int64
	if err := scanner.Scan(
		&item.ID, &item.Source, &item.TaskID, &item.ReleaseOrderID, &item.OrderNo, &item.ApplicationID,
		&item.ApplicationName, &item.EnvCode, &item.OperationType, &item.TriggeredBy, &item.FlowName, &item.NodeName,
		&item.Gate, &item.ExecutionScope, &item.Action, &item.OperatorUserID, &item.OperatorName, &item.Comment,
		&item.ReleaseOrderStatus, &createdAt,
	); err != nil {
		return item, err
	}
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	return item, nil
}

func marshalApprovalFlowDefinition(nodes []domain.ApprovalFlowNode, links []domain.ApprovalFlowLink) (string, error) {
	encoded, err := json.Marshal(struct {
		Nodes []domain.ApprovalFlowNode `json:"nodes"`
		Links []domain.ApprovalFlowLink `json:"links"`
	}{Nodes: nodes, Links: links})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func scanApprovalFlowDefinition(scanner interface{ Scan(...any) error }) (domain.ApprovalFlowDefinition, error) {
	var item domain.ApprovalFlowDefinition
	var nodesJSON string
	var createdAt, updatedAt int64
	if err := scanner.Scan(&item.ID, &item.Name, &item.Status, &nodesJSON, &createdAt, &updatedAt); err != nil {
		return item, err
	}
	if strings.HasPrefix(strings.TrimSpace(nodesJSON), "[") {
		if err := json.Unmarshal([]byte(nodesJSON), &item.Nodes); err != nil {
			return item, err
		}
	} else if err := json.Unmarshal([]byte(nodesJSON), &struct {
		Nodes *[]domain.ApprovalFlowNode `json:"nodes"`
		Links *[]domain.ApprovalFlowLink `json:"links"`
	}{Nodes: &item.Nodes, Links: &item.Links}); err != nil {
		return item, err
	}
	item.CreatedAt, item.UpdatedAt = time.Unix(0, createdAt).UTC(), time.Unix(0, updatedAt).UTC()
	return item, nil
}

func scanApprovalFlowInstance(scanner interface{ Scan(...any) error }) (domain.ReleaseOrderApprovalFlowInstance, error) {
	var item domain.ReleaseOrderApprovalFlowInstance
	var nodesJSON string
	var createdAt, updatedAt int64
	if err := scanner.Scan(&item.ID, &item.ReleaseOrderID, &item.FlowDefinitionID, &item.FlowName, &nodesJSON, &item.Status, &item.CurrentGate, &item.CurrentScope, &item.CurrentNodeCode, &item.CurrentTaskID, &createdAt, &updatedAt); err != nil {
		return item, err
	}
	if strings.HasPrefix(strings.TrimSpace(nodesJSON), "[") {
		if err := json.Unmarshal([]byte(nodesJSON), &item.Nodes); err != nil {
			return item, err
		}
	} else if err := json.Unmarshal([]byte(nodesJSON), &struct {
		Nodes *[]domain.ApprovalFlowNode `json:"nodes"`
		Links *[]domain.ApprovalFlowLink `json:"links"`
	}{Nodes: &item.Nodes, Links: &item.Links}); err != nil {
		return item, err
	}
	item.CreatedAt, item.UpdatedAt = time.Unix(0, createdAt).UTC(), time.Unix(0, updatedAt).UTC()
	return item, nil
}

func scanApprovalFlowTask(scanner interface{ Scan(...any) error }) (domain.ReleaseOrderApprovalFlowTask, error) {
	var item domain.ReleaseOrderApprovalFlowTask
	var idsJSON, namesJSON string
	var createdAt, updatedAt int64
	if err := scanner.Scan(
		&item.ID, &item.InstanceID, &item.ReleaseOrderID, &item.NodeCode, &item.NodeName, &item.Gate, &item.NodeType,
		&item.ApprovalMode, &idsJSON, &namesJSON, &item.AgentTaskID, &item.AgentTaskName, &item.AgentBatchID, &item.Message,
		&item.Status, &createdAt, &updatedAt,
	); err != nil {
		return item, err
	}
	if item.NodeType == "" {
		item.NodeType = domain.ApprovalFlowNodeTypeApproval
	}
	if err := json.Unmarshal([]byte(idsJSON), &item.ApproverIDs); err != nil {
		return item, err
	}
	if err := json.Unmarshal([]byte(namesJSON), &item.ApproverNames); err != nil {
		return item, err
	}
	item.CreatedAt, item.UpdatedAt = time.Unix(0, createdAt).UTC(), time.Unix(0, updatedAt).UTC()
	return item, nil
}
