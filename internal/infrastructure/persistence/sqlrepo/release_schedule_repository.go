package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/release"
)

func (r *ReleaseRepository) initScheduleSchema(ctx context.Context) error {
	statements, err := releaseScheduleSchemaStatements(r.dbDriver)
	if err != nil {
		return err
	}
	for _, stmt := range statements {
		if _, execErr := r.db.ExecContext(ctx, stmt); execErr != nil {
			return execErr
		}
	}
	return nil
}

func releaseScheduleSchemaStatements(dbDriver string) ([]string, error) {
	switch dbDriver {
	case "mysql":
		return []string{
			`CREATE TABLE IF NOT EXISTS release_order_schedule (
	id VARCHAR(64) PRIMARY KEY,
	schedule_no VARCHAR(64) NOT NULL,
	release_order_id VARCHAR(64) NOT NULL,
	release_order_no VARCHAR(64) NOT NULL DEFAULT '',
	application_id VARCHAR(64) NOT NULL,
	application_name VARCHAR(128) NOT NULL DEFAULT '',
	env_code VARCHAR(64) NOT NULL DEFAULT '',
	template_id VARCHAR(64) NOT NULL DEFAULT '',
	template_name VARCHAR(128) NOT NULL DEFAULT '',
	schedule_mode VARCHAR(32) NOT NULL,
	build_scheduled_at BIGINT NULL,
	deploy_scheduled_at BIGINT NULL,
	execute_scheduled_at BIGINT NULL,
	cd_conflict_at BIGINT NULL,
	timezone VARCHAR(64) NOT NULL DEFAULT '',
	status VARCHAR(32) NOT NULL,
	approval_required TINYINT(1) NOT NULL DEFAULT 0,
	approval_mode VARCHAR(32) NOT NULL DEFAULT '',
	approval_approver_ids_json TEXT NOT NULL,
	approval_approver_names_json TEXT NOT NULL,
	approved_at BIGINT NULL,
	approved_by VARCHAR(128) NOT NULL DEFAULT '',
	rejected_at BIGINT NULL,
	rejected_by VARCHAR(128) NOT NULL DEFAULT '',
	rejected_reason VARCHAR(1000) NOT NULL DEFAULT '',
	build_dispatched_at BIGINT NULL,
	deploy_dispatched_at BIGINT NULL,
	execute_dispatched_at BIGINT NULL,
	expired_at BIGINT NULL,
	cancelled_at BIGINT NULL,
	cancelled_by VARCHAR(128) NOT NULL DEFAULT '',
	last_error VARCHAR(1000) NOT NULL DEFAULT '',
	remark VARCHAR(500) NOT NULL DEFAULT '',
	creator_user_id VARCHAR(64) NOT NULL DEFAULT '',
	creator_name VARCHAR(128) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_release_order_schedule_no (schedule_no),
	KEY idx_release_order_schedule_order_status (release_order_id, status),
	KEY idx_release_order_schedule_cd_conflict (application_id, env_code, cd_conflict_at, status),
	KEY idx_release_order_schedule_status_build (status, build_scheduled_at),
	KEY idx_release_order_schedule_status_deploy (status, deploy_scheduled_at),
	KEY idx_release_order_schedule_status_execute (status, execute_scheduled_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS release_order_schedule_approval_record (
	id VARCHAR(64) PRIMARY KEY,
	schedule_id VARCHAR(64) NOT NULL,
	action VARCHAR(32) NOT NULL,
	operator_user_id VARCHAR(64) NOT NULL DEFAULT '',
	operator_name VARCHAR(100) NOT NULL DEFAULT '',
	comment VARCHAR(1000) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	KEY idx_release_order_schedule_approval_record_schedule_created (schedule_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}, nil
	case "sqlite":
		return []string{
			`CREATE TABLE IF NOT EXISTS release_order_schedule (
	id TEXT PRIMARY KEY,
	schedule_no TEXT NOT NULL UNIQUE,
	release_order_id TEXT NOT NULL,
	release_order_no TEXT NOT NULL DEFAULT '',
	application_id TEXT NOT NULL,
	application_name TEXT NOT NULL DEFAULT '',
	env_code TEXT NOT NULL DEFAULT '',
	template_id TEXT NOT NULL DEFAULT '',
	template_name TEXT NOT NULL DEFAULT '',
	schedule_mode TEXT NOT NULL,
	build_scheduled_at INTEGER NULL,
	deploy_scheduled_at INTEGER NULL,
	execute_scheduled_at INTEGER NULL,
	cd_conflict_at INTEGER NULL,
	timezone TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	approval_required INTEGER NOT NULL DEFAULT 0,
	approval_mode TEXT NOT NULL DEFAULT '',
	approval_approver_ids_json TEXT NOT NULL DEFAULT '[]',
	approval_approver_names_json TEXT NOT NULL DEFAULT '[]',
	approved_at INTEGER NULL,
	approved_by TEXT NOT NULL DEFAULT '',
	rejected_at INTEGER NULL,
	rejected_by TEXT NOT NULL DEFAULT '',
	rejected_reason TEXT NOT NULL DEFAULT '',
	build_dispatched_at INTEGER NULL,
	deploy_dispatched_at INTEGER NULL,
	execute_dispatched_at INTEGER NULL,
	expired_at INTEGER NULL,
	cancelled_at INTEGER NULL,
	cancelled_by TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	remark TEXT NOT NULL DEFAULT '',
	creator_user_id TEXT NOT NULL DEFAULT '',
	creator_name TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_release_order_schedule_order_status ON release_order_schedule (release_order_id, status);`,
			`CREATE INDEX IF NOT EXISTS idx_release_order_schedule_cd_conflict ON release_order_schedule (application_id, env_code, cd_conflict_at, status);`,
			`CREATE INDEX IF NOT EXISTS idx_release_order_schedule_status_build ON release_order_schedule (status, build_scheduled_at);`,
			`CREATE INDEX IF NOT EXISTS idx_release_order_schedule_status_deploy ON release_order_schedule (status, deploy_scheduled_at);`,
			`CREATE INDEX IF NOT EXISTS idx_release_order_schedule_status_execute ON release_order_schedule (status, execute_scheduled_at);`,
			`CREATE TABLE IF NOT EXISTS release_order_schedule_approval_record (
	id TEXT PRIMARY KEY,
	schedule_id TEXT NOT NULL,
	action TEXT NOT NULL,
	operator_user_id TEXT NOT NULL DEFAULT '',
	operator_name TEXT NOT NULL DEFAULT '',
	comment TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_release_order_schedule_approval_record_schedule_created ON release_order_schedule_approval_record (schedule_id, created_at);`,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported db driver: %s", dbDriver)
	}
}

func (r *ReleaseRepository) CreateSchedule(ctx context.Context, item domain.ReleaseOrderSchedule) error {
	const q = `
INSERT INTO release_order_schedule (
	id, schedule_no, release_order_id, release_order_no, application_id, application_name, env_code, template_id, template_name, schedule_mode,
	build_scheduled_at, deploy_scheduled_at, execute_scheduled_at, cd_conflict_at, timezone, status, approval_required, approval_mode,
	approval_approver_ids_json, approval_approver_names_json, approved_at, approved_by, rejected_at, rejected_by, rejected_reason,
	build_dispatched_at, deploy_dispatched_at, execute_dispatched_at, expired_at, cancelled_at, cancelled_by, last_error, remark,
	creator_user_id, creator_name, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	_, err := r.db.ExecContext(
		ctx,
		q,
		item.ID,
		item.ScheduleNo,
		item.ReleaseOrderID,
		item.ReleaseOrderNo,
		item.ApplicationID,
		item.ApplicationName,
		item.EnvCode,
		item.TemplateID,
		item.TemplateName,
		string(item.ScheduleMode),
		nullableUnixNano(item.BuildScheduledAt),
		nullableUnixNano(item.DeployScheduledAt),
		nullableUnixNano(item.ExecuteScheduledAt),
		nullableUnixNano(item.CDConflictAt),
		item.Timezone,
		string(item.Status),
		boolToDBValue(r.dbDriver, item.ApprovalRequired),
		string(item.ApprovalMode),
		marshalStringSlice(item.ApprovalApproverIDs),
		marshalStringSlice(item.ApprovalApproverNames),
		nullableUnixNano(item.ApprovedAt),
		item.ApprovedBy,
		nullableUnixNano(item.RejectedAt),
		item.RejectedBy,
		item.RejectedReason,
		nullableUnixNano(item.BuildDispatchedAt),
		nullableUnixNano(item.DeployDispatchedAt),
		nullableUnixNano(item.ExecuteDispatchedAt),
		nullableUnixNano(item.ExpiredAt),
		nullableUnixNano(item.CancelledAt),
		item.CancelledBy,
		item.LastError,
		item.Remark,
		item.CreatorUserID,
		item.CreatorName,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.ErrScheduleDuplicated
		}
		return err
	}
	return nil
}

func (r *ReleaseRepository) UpdateSchedule(ctx context.Context, item domain.ReleaseOrderSchedule) error {
	const q = `
UPDATE release_order_schedule
SET schedule_no = ?, release_order_id = ?, release_order_no = ?, application_id = ?, application_name = ?, env_code = ?, template_id = ?, template_name = ?,
	schedule_mode = ?, build_scheduled_at = ?, deploy_scheduled_at = ?, execute_scheduled_at = ?, cd_conflict_at = ?, timezone = ?, status = ?,
	approval_required = ?, approval_mode = ?, approval_approver_ids_json = ?, approval_approver_names_json = ?, approved_at = ?, approved_by = ?,
	rejected_at = ?, rejected_by = ?, rejected_reason = ?, build_dispatched_at = ?, deploy_dispatched_at = ?, execute_dispatched_at = ?,
	expired_at = ?, cancelled_at = ?, cancelled_by = ?, last_error = ?, remark = ?, creator_user_id = ?, creator_name = ?, updated_at = ?
WHERE id = ?;`

	res, err := r.db.ExecContext(
		ctx,
		q,
		item.ScheduleNo,
		item.ReleaseOrderID,
		item.ReleaseOrderNo,
		item.ApplicationID,
		item.ApplicationName,
		item.EnvCode,
		item.TemplateID,
		item.TemplateName,
		string(item.ScheduleMode),
		nullableUnixNano(item.BuildScheduledAt),
		nullableUnixNano(item.DeployScheduledAt),
		nullableUnixNano(item.ExecuteScheduledAt),
		nullableUnixNano(item.CDConflictAt),
		item.Timezone,
		string(item.Status),
		boolToDBValue(r.dbDriver, item.ApprovalRequired),
		string(item.ApprovalMode),
		marshalStringSlice(item.ApprovalApproverIDs),
		marshalStringSlice(item.ApprovalApproverNames),
		nullableUnixNano(item.ApprovedAt),
		item.ApprovedBy,
		nullableUnixNano(item.RejectedAt),
		item.RejectedBy,
		item.RejectedReason,
		nullableUnixNano(item.BuildDispatchedAt),
		nullableUnixNano(item.DeployDispatchedAt),
		nullableUnixNano(item.ExecuteDispatchedAt),
		nullableUnixNano(item.ExpiredAt),
		nullableUnixNano(item.CancelledAt),
		item.CancelledBy,
		item.LastError,
		item.Remark,
		item.CreatorUserID,
		item.CreatorName,
		item.UpdatedAt.UTC().UnixNano(),
		item.ID,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrScheduleNotFound
	}
	return nil
}

func (r *ReleaseRepository) GetScheduleByID(ctx context.Context, id string) (domain.ReleaseOrderSchedule, error) {
	query := releaseOrderScheduleSelectSQL() + ` WHERE id = ?;`
	return r.getSingleSchedule(ctx, query, strings.TrimSpace(id))
}

func (r *ReleaseRepository) GetActiveScheduleByOrderID(ctx context.Context, releaseOrderID string) (domain.ReleaseOrderSchedule, error) {
	query := releaseOrderScheduleSelectSQL() + `
WHERE release_order_id = ?
  AND status IN (?, ?, ?, ?)
ORDER BY updated_at DESC, created_at DESC
LIMIT 1;`
	return r.getSingleSchedule(
		ctx,
		query,
		strings.TrimSpace(releaseOrderID),
		string(domain.ScheduleStatusPendingApproval),
		string(domain.ScheduleStatusApproving),
		string(domain.ScheduleStatusScheduled),
		string(domain.ScheduleStatusDispatching),
	)
}

func (r *ReleaseRepository) FindActiveScheduleCDConflict(
	ctx context.Context,
	applicationID string,
	envCode string,
	cdConflictAt time.Time,
	excludeScheduleID string,
) (domain.ReleaseOrderSchedule, error) {
	query := releaseOrderScheduleSelectSQL() + `
WHERE application_id = ?
  AND env_code = ?
  AND cd_conflict_at = ?
  AND status IN (?, ?, ?, ?)`
	args := []any{
		strings.TrimSpace(applicationID),
		strings.TrimSpace(envCode),
		cdConflictAt.UTC().UnixNano(),
		string(domain.ScheduleStatusPendingApproval),
		string(domain.ScheduleStatusApproving),
		string(domain.ScheduleStatusScheduled),
		string(domain.ScheduleStatusDispatching),
	}
	if value := strings.TrimSpace(excludeScheduleID); value != "" {
		query += " AND id <> ?"
		args = append(args, value)
	}
	query += " ORDER BY created_at ASC LIMIT 1;"
	return r.getSingleSchedule(ctx, query, args...)
}

func (r *ReleaseRepository) ListSchedules(ctx context.Context, filter domain.ScheduleListFilter) ([]domain.ReleaseOrderSchedule, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	where, args := buildScheduleListWhere(filter)
	countQuery := `SELECT COUNT(1) FROM release_order_schedule`
	if len(where) > 0 {
		countQuery += " WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := releaseOrderScheduleSelectSQL()
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?;"
	offset := (filter.Page - 1) * filter.PageSize
	rows, err := r.db.QueryContext(ctx, query, append(args, filter.PageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanReleaseOrderSchedules(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *ReleaseRepository) ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]domain.ReleaseOrderSchedule, error) {
	if limit <= 0 {
		limit = 50
	}
	query := releaseOrderScheduleSelectSQL() + `
WHERE (
    status IN (?, ?)
    AND (
      (build_scheduled_at IS NOT NULL AND build_scheduled_at <= ?)
      OR (deploy_scheduled_at IS NOT NULL AND deploy_scheduled_at <= ?)
      OR (execute_scheduled_at IS NOT NULL AND execute_scheduled_at <= ?)
    )
  )
  OR (
    status = ?
    AND (
      (build_scheduled_at IS NOT NULL AND build_scheduled_at <= ? AND build_dispatched_at IS NULL)
      OR (deploy_scheduled_at IS NOT NULL AND deploy_scheduled_at <= ? AND deploy_dispatched_at IS NULL)
      OR (execute_scheduled_at IS NOT NULL AND execute_scheduled_at <= ? AND execute_dispatched_at IS NULL)
    )
  )
ORDER BY updated_at ASC, created_at ASC
LIMIT ?;`
	nowNs := now.UTC().UnixNano()
	rows, err := r.db.QueryContext(
		ctx,
		query,
		string(domain.ScheduleStatusPendingApproval),
		string(domain.ScheduleStatusApproving),
		nowNs,
		nowNs,
		nowNs,
		string(domain.ScheduleStatusScheduled),
		nowNs,
		nowNs,
		nowNs,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReleaseOrderSchedules(rows)
}

func (r *ReleaseRepository) UpdateScheduleStatus(
	ctx context.Context,
	id string,
	status domain.ScheduleStatus,
	lastError string,
	updatedAt time.Time,
) (domain.ReleaseOrderSchedule, error) {
	const q = `
UPDATE release_order_schedule
SET status = ?, last_error = ?, updated_at = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, string(status), strings.TrimSpace(lastError), updatedAt.UTC().UnixNano(), strings.TrimSpace(id))
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if affected == 0 {
		return domain.ReleaseOrderSchedule{}, domain.ErrScheduleNotFound
	}
	return r.GetScheduleByID(ctx, id)
}

func (r *ReleaseRepository) CreateScheduleApprovalRecord(ctx context.Context, item domain.ReleaseOrderScheduleApprovalRecord) error {
	const q = `
INSERT INTO release_order_schedule_approval_record (
	id, schedule_id, action, operator_user_id, operator_name, comment, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(
		ctx,
		q,
		item.ID,
		item.ScheduleID,
		string(item.Action),
		strings.TrimSpace(item.OperatorUserID),
		strings.TrimSpace(item.OperatorName),
		strings.TrimSpace(item.Comment),
		item.CreatedAt.UTC().UnixNano(),
	)
	return err
}

func (r *ReleaseRepository) ListScheduleApprovalRecords(ctx context.Context, scheduleID string) ([]domain.ReleaseOrderScheduleApprovalRecord, error) {
	const q = `
SELECT id, schedule_id, action, operator_user_id, operator_name, comment, created_at
FROM release_order_schedule_approval_record
WHERE schedule_id = ?
ORDER BY created_at ASC, id ASC;`
	rows, err := r.db.QueryContext(ctx, q, strings.TrimSpace(scheduleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.ReleaseOrderScheduleApprovalRecord, 0)
	for rows.Next() {
		item, scanErr := scanReleaseOrderScheduleApprovalRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *ReleaseRepository) getSingleSchedule(ctx context.Context, query string, args ...any) (domain.ReleaseOrderSchedule, error) {
	item, err := scanReleaseOrderSchedule(r.db.QueryRowContext(ctx, query, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ReleaseOrderSchedule{}, domain.ErrScheduleNotFound
		}
		return domain.ReleaseOrderSchedule{}, err
	}
	return item, nil
}

func buildScheduleListWhere(filter domain.ScheduleListFilter) ([]string, []any) {
	where := make([]string, 0, 8)
	args := make([]any, 0, 12)
	if value := strings.TrimSpace(filter.ApplicationID); value != "" {
		where = append(where, "application_id = ?")
		args = append(args, value)
	}
	if len(filter.ApplicationIDs) > 0 {
		placeholders := make([]string, 0, len(filter.ApplicationIDs))
		for _, item := range filter.ApplicationIDs {
			value := strings.TrimSpace(item)
			if value == "" {
				continue
			}
			placeholders = append(placeholders, "?")
			args = append(args, value)
		}
		if len(placeholders) > 0 {
			where = append(where, "application_id IN ("+strings.Join(placeholders, ", ")+")")
		}
	}
	if value := strings.TrimSpace(filter.EnvCode); value != "" {
		where = append(where, "env_code = ?")
		args = append(args, value)
	}
	if filter.ScheduleMode != "" {
		where = append(where, "schedule_mode = ?")
		args = append(args, string(filter.ScheduleMode))
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}
	if value := strings.TrimSpace(filter.CreatorUserID); value != "" {
		where = append(where, "creator_user_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.ApprovalApproverUserID); value != "" {
		where = append(where, "approval_approver_ids_json LIKE ?")
		args = append(args, "%\""+value+"\"%")
	}
	if value := strings.TrimSpace(filter.VisibleToUserID); value != "" {
		where = append(where, "(creator_user_id = ? OR approval_approver_ids_json LIKE ?)")
		args = append(args, value, "%\""+value+"\"%")
	}
	if value := strings.TrimSpace(filter.Keyword); value != "" {
		like := "%" + value + "%"
		where = append(where, "(schedule_no LIKE ? OR release_order_no LIKE ? OR application_name LIKE ?)")
		args = append(args, like, like, like)
	}
	if filter.ScheduledAtFrom != nil {
		where = append(where, "COALESCE(build_scheduled_at, deploy_scheduled_at, execute_scheduled_at) >= ?")
		args = append(args, filter.ScheduledAtFrom.UTC().UnixNano())
	}
	if filter.ScheduledAtTo != nil {
		where = append(where, "COALESCE(build_scheduled_at, deploy_scheduled_at, execute_scheduled_at) <= ?")
		args = append(args, filter.ScheduledAtTo.UTC().UnixNano())
	}
	return where, args
}

func releaseOrderScheduleSelectSQL() string {
	return `
SELECT id, schedule_no, release_order_id, release_order_no, application_id, application_name, env_code, template_id, template_name,
	schedule_mode, build_scheduled_at, deploy_scheduled_at, execute_scheduled_at, cd_conflict_at, timezone, status, approval_required,
	approval_mode, approval_approver_ids_json, approval_approver_names_json, approved_at, approved_by, rejected_at, rejected_by,
	rejected_reason, build_dispatched_at, deploy_dispatched_at, execute_dispatched_at, expired_at, cancelled_at, cancelled_by,
	last_error, remark, creator_user_id, creator_name, created_at, updated_at
FROM release_order_schedule`
}

func scanReleaseOrderSchedules(rows *sql.Rows) ([]domain.ReleaseOrderSchedule, error) {
	items := make([]domain.ReleaseOrderSchedule, 0)
	for rows.Next() {
		item, err := scanReleaseOrderSchedule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func scanReleaseOrderSchedule(s scanner) (domain.ReleaseOrderSchedule, error) {
	var (
		item                domain.ReleaseOrderSchedule
		scheduleMode        string
		status              string
		approvalRequired    any
		approvalMode        string
		approverIDsJSON     string
		approverNamesJSON   string
		buildScheduledAt    sql.NullInt64
		deployScheduledAt   sql.NullInt64
		executeScheduledAt  sql.NullInt64
		cdConflictAt        sql.NullInt64
		approvedAt          sql.NullInt64
		rejectedAt          sql.NullInt64
		buildDispatchedAt   sql.NullInt64
		deployDispatchedAt  sql.NullInt64
		executeDispatchedAt sql.NullInt64
		expiredAt           sql.NullInt64
		cancelledAt         sql.NullInt64
		createdAt           int64
		updatedAt           int64
	)
	if err := s.Scan(
		&item.ID,
		&item.ScheduleNo,
		&item.ReleaseOrderID,
		&item.ReleaseOrderNo,
		&item.ApplicationID,
		&item.ApplicationName,
		&item.EnvCode,
		&item.TemplateID,
		&item.TemplateName,
		&scheduleMode,
		&buildScheduledAt,
		&deployScheduledAt,
		&executeScheduledAt,
		&cdConflictAt,
		&item.Timezone,
		&status,
		&approvalRequired,
		&approvalMode,
		&approverIDsJSON,
		&approverNamesJSON,
		&approvedAt,
		&item.ApprovedBy,
		&rejectedAt,
		&item.RejectedBy,
		&item.RejectedReason,
		&buildDispatchedAt,
		&deployDispatchedAt,
		&executeDispatchedAt,
		&expiredAt,
		&cancelledAt,
		&item.CancelledBy,
		&item.LastError,
		&item.Remark,
		&item.CreatorUserID,
		&item.CreatorName,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	item.ScheduleMode = domain.ScheduleMode(scheduleMode)
	item.Status = domain.ScheduleStatus(status)
	item.ApprovalRequired = scanBoolValue(approvalRequired)
	item.ApprovalMode = domain.TemplateApprovalMode(approvalMode)
	item.ApprovalApproverIDs = unmarshalStringSlice(approverIDsJSON)
	item.ApprovalApproverNames = unmarshalStringSlice(approverNamesJSON)
	item.BuildScheduledAt = nullUnixNanoTime(buildScheduledAt)
	item.DeployScheduledAt = nullUnixNanoTime(deployScheduledAt)
	item.ExecuteScheduledAt = nullUnixNanoTime(executeScheduledAt)
	item.CDConflictAt = nullUnixNanoTime(cdConflictAt)
	item.ApprovedAt = nullUnixNanoTime(approvedAt)
	item.RejectedAt = nullUnixNanoTime(rejectedAt)
	item.BuildDispatchedAt = nullUnixNanoTime(buildDispatchedAt)
	item.DeployDispatchedAt = nullUnixNanoTime(deployDispatchedAt)
	item.ExecuteDispatchedAt = nullUnixNanoTime(executeDispatchedAt)
	item.ExpiredAt = nullUnixNanoTime(expiredAt)
	item.CancelledAt = nullUnixNanoTime(cancelledAt)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

func scanReleaseOrderScheduleApprovalRecord(s scanner) (domain.ReleaseOrderScheduleApprovalRecord, error) {
	var (
		item      domain.ReleaseOrderScheduleApprovalRecord
		actionRaw string
		createdAt int64
	)
	if err := s.Scan(
		&item.ID,
		&item.ScheduleID,
		&actionRaw,
		&item.OperatorUserID,
		&item.OperatorName,
		&item.Comment,
		&createdAt,
	); err != nil {
		return domain.ReleaseOrderScheduleApprovalRecord{}, err
	}
	item.Action = domain.ReleaseOrderApprovalAction(actionRaw)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	return item, nil
}

func nullUnixNanoTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	t := time.Unix(0, value.Int64).UTC()
	return &t
}
