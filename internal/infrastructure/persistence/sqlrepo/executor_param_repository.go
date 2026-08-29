package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/executorparam"
	pipelinedomain "gos/internal/domain/pipeline"
)

type ExecutorParamRepository struct {
	db       *sql.DB
	dbDriver string
}

// NewExecutorParamRepository 统一承接“执行器参数定义”的持久化。
//
// 这里虽然是一次命名升级，但必须兼容线上已存在的旧表 `pipeline_param_def`。
// 因此仓储初始化阶段会主动做表名迁移，保证旧数据无需手工导出导入。
func NewExecutorParamRepository(db *sql.DB, dbDriver string) *ExecutorParamRepository {
	return &ExecutorParamRepository{
		db:       db,
		dbDriver: strings.ToLower(strings.TrimSpace(dbDriver)),
	}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) InitSchema(ctx context.Context) error {
	// 兼容旧版本表名：
	// 如果数据库里还是 `pipeline_param_def`，必须先迁移到新表名，
	// 否则后续 `CREATE TABLE IF NOT EXISTS executor_param_def` 会先建空表，
	// 导致旧数据仍留在老表里但新代码读不到。
	if err := r.renameLegacyTable(ctx, "pipeline_param_def", "executor_param_def"); err != nil {
		return err
	}

	statements, err := r.schemaStatements()
	if err != nil {
		return err
	}
	for _, stmt := range statements {
		if _, execErr := r.db.ExecContext(ctx, stmt); execErr != nil {
			return execErr
		}
	}
	return r.migrateSchema(ctx)
}

// schemaStatements 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) schemaStatements() ([]string, error) {
	switch r.dbDriver {
	case "mysql":
		return []string{
			`CREATE TABLE IF NOT EXISTS executor_param_def (
	id VARCHAR(64) PRIMARY KEY,
	pipeline_id VARCHAR(64) NOT NULL,
	executor_type VARCHAR(50) NOT NULL,
	executor_param_name VARCHAR(100) NOT NULL,
	param_key VARCHAR(100) NOT NULL DEFAULT '',
	param_type VARCHAR(50) NOT NULL,
	single_select TINYINT(1) NOT NULL DEFAULT 0,
	required TINYINT(1) NOT NULL,
	default_value VARCHAR(500) NOT NULL,
	description VARCHAR(500) NOT NULL,
	visible TINYINT(1) NOT NULL,
	editable TINYINT(1) NOT NULL,
	source_from VARCHAR(50) NOT NULL,
	status VARCHAR(32) NOT NULL DEFAULT 'active',
	raw_meta JSON NULL,
	sort_no INT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_pipeline_param_unique (pipeline_id, executor_type, executor_param_name),
	KEY idx_pipeline_param_pipeline_sort (pipeline_id, sort_no),
	KEY idx_pipeline_param_param_key (param_key),
	KEY idx_pipeline_param_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}, nil
	case "sqlite":
		return []string{
			`CREATE TABLE IF NOT EXISTS executor_param_def (
	id TEXT PRIMARY KEY,
	pipeline_id TEXT NOT NULL,
	executor_type TEXT NOT NULL,
	executor_param_name TEXT NOT NULL,
	param_key TEXT NOT NULL DEFAULT '',
	param_type TEXT NOT NULL,
	single_select INTEGER NOT NULL DEFAULT 0,
	required INTEGER NOT NULL,
	default_value TEXT NOT NULL,
	description TEXT NOT NULL,
	visible INTEGER NOT NULL,
	editable INTEGER NOT NULL,
	source_from TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	raw_meta TEXT NOT NULL,
	sort_no INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE(pipeline_id, executor_type, executor_param_name)
);`,
			`CREATE INDEX IF NOT EXISTS idx_pipeline_param_pipeline_sort ON executor_param_def (pipeline_id, sort_no);`,
			`CREATE INDEX IF NOT EXISTS idx_pipeline_param_param_key ON executor_param_def (param_key);`,
			`CREATE INDEX IF NOT EXISTS idx_pipeline_param_status ON executor_param_def (status);`,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

// migrateSchema 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) migrateSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
		type columnDef struct {
			name string
			ddl  string
		}
		additions := []columnDef{
			{
				name: "single_select",
				ddl:  `ALTER TABLE executor_param_def ADD COLUMN single_select TINYINT(1) NOT NULL DEFAULT 0 AFTER param_type;`,
			},
			{
				name: "status",
				ddl:  `ALTER TABLE executor_param_def ADD COLUMN status VARCHAR(32) NOT NULL DEFAULT 'active' AFTER source_from;`,
			},
		}
		for _, item := range additions {
			exists, err := r.mysqlColumnExists(ctx, "executor_param_def", item.name)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			if _, err := r.db.ExecContext(ctx, item.ddl); err != nil {
				return err
			}
		}
		return nil
	case "sqlite":
		columns, err := r.sqliteTableColumns(ctx, "executor_param_def")
		if err != nil {
			return err
		}
		if _, ok := columns["single_select"]; !ok {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE executor_param_def ADD COLUMN single_select INTEGER NOT NULL DEFAULT 0;`,
			); err != nil {
				return err
			}
		}
		if _, ok := columns["status"]; !ok {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE executor_param_def ADD COLUMN status TEXT NOT NULL DEFAULT 'active';`,
			); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

// renameLegacyTable 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) renameLegacyTable(ctx context.Context, legacyTable, targetTable string) error {
	targetExists, err := r.tableExists(ctx, targetTable)
	if err != nil {
		return err
	}
	if targetExists {
		return nil
	}

	legacyExists, err := r.tableExists(ctx, legacyTable)
	if err != nil {
		return err
	}
	if !legacyExists {
		return nil
	}

	switch r.dbDriver {
	case "mysql":
		_, err = r.db.ExecContext(ctx, fmt.Sprintf("RENAME TABLE %s TO %s;", legacyTable, targetTable))
		return err
	case "sqlite":
		_, err = r.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", legacyTable, targetTable))
		return err
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

// tableExists 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) tableExists(ctx context.Context, table string) (bool, error) {
	switch r.dbDriver {
	case "mysql":
		const q = `
SELECT COUNT(1)
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?;`
		var count int
		if err := r.db.QueryRowContext(ctx, q, table).Scan(&count); err != nil {
			return false, err
		}
		return count > 0, nil
	case "sqlite":
		const q = `SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?;`
		var count int
		if err := r.db.QueryRowContext(ctx, q, table).Scan(&count); err != nil {
			return false, err
		}
		return count > 0, nil
	default:
		return false, fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

// mysqlColumnExists 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) mysqlColumnExists(ctx context.Context, table, column string) (bool, error) {
	const q = `
SELECT COUNT(1)
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?;`

	var count int
	if err := r.db.QueryRowContext(ctx, q, table, column).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// sqliteTableColumns 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
	q := fmt.Sprintf("PRAGMA table_info(%q);", table)
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notNull   int
			defaultV  sql.NullString
			primaryID int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryID); err != nil {
			return nil, err
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

// Upsert 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) Upsert(ctx context.Context, items []domain.ExecutorParamDef) (int, int, error) {
	if r.dbDriver == "mysql" {
		return r.upsertMySQL(ctx, items)
	}

	const (
		updateByKey = `UPDATE executor_param_def
SET param_key = CASE WHEN TRIM(param_key) = '' THEN ? ELSE param_key END,
	param_type = ?, single_select = ?, required = ?, default_value = ?, description = ?, visible = ?, editable = ?, source_from = ?, status = ?, raw_meta = ?, sort_no = ?, updated_at = ?
WHERE pipeline_id = ? AND executor_type = ? AND executor_param_name = ?;`
		insert = `INSERT INTO executor_param_def (
	id, pipeline_id, executor_type, executor_param_name, param_key, param_type, single_select, required, default_value, description, visible, editable, source_from, status, raw_meta, sort_no, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	)

	created := 0
	updated := 0
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return created, updated, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	updateStmt, err := tx.PrepareContext(ctx, updateByKey)
	if err != nil {
		return created, updated, err
	}
	defer func() { _ = updateStmt.Close() }()

	insertStmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return created, updated, err
	}
	defer func() { _ = insertStmt.Close() }()

	for _, item := range items {
		res, err := updateStmt.ExecContext(
			ctx,
			item.ParamKey,
			string(item.ParamType),
			boolToInt(item.SingleSelect),
			boolToInt(item.Required),
			item.DefaultValue,
			item.Description,
			boolToInt(item.Visible),
			boolToInt(item.Editable),
			string(item.SourceFrom),
			string(item.Status),
			item.RawMeta,
			item.SortNo,
			item.UpdatedAt.UTC().UnixNano(),
			item.PipelineID,
			string(item.ExecutorType),
			item.ExecutorParamName,
		)
		if err != nil {
			return created, updated, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return created, updated, err
		}
		if affected > 0 {
			updated++
			continue
		}

		_, err = insertStmt.ExecContext(
			ctx,
			item.ID,
			item.PipelineID,
			string(item.ExecutorType),
			item.ExecutorParamName,
			item.ParamKey,
			string(item.ParamType),
			boolToInt(item.SingleSelect),
			boolToInt(item.Required),
			item.DefaultValue,
			item.Description,
			boolToInt(item.Visible),
			boolToInt(item.Editable),
			string(item.SourceFrom),
			string(item.Status),
			item.RawMeta,
			item.SortNo,
			item.CreatedAt.UTC().UnixNano(),
			item.UpdatedAt.UTC().UnixNano(),
		)
		if err != nil {
			if isDuplicateKeyError(r.dbDriver, err) {
				updated++
				continue
			}
			return created, updated, err
		}
		created++
	}

	if err := tx.Commit(); err != nil {
		return created, updated, err
	}
	return created, updated, nil
}

// upsertMySQL 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) upsertMySQL(ctx context.Context, items []domain.ExecutorParamDef) (int, int, error) {
	if len(items) == 0 {
		return 0, 0, nil
	}

	existingKeys, err := r.mysqlExistingParamKeys(ctx, items)
	if err != nil {
		return 0, 0, err
	}

	created := 0
	updated := 0
	for _, item := range items {
		if _, ok := existingKeys[executorParamUniqueKey(item.PipelineID, item.ExecutorType, item.ExecutorParamName)]; ok {
			updated++
			continue
		}
		created++
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const batchSize = 200
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		if err := r.mysqlBatchUpsert(ctx, tx, items[start:end]); err != nil {
			return 0, 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return created, updated, nil
}

// mysqlBatchUpsert 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) mysqlBatchUpsert(ctx context.Context, tx *sql.Tx, items []domain.ExecutorParamDef) error {
	if len(items) == 0 {
		return nil
	}

	var builder strings.Builder
	builder.WriteString(`INSERT INTO executor_param_def (
id, pipeline_id, executor_type, executor_param_name, param_key, param_type, single_select, required, default_value, description, visible, editable, source_from, status, raw_meta, sort_no, created_at, updated_at
) VALUES `)

	args := make([]any, 0, len(items)*18)
	for idx, item := range items {
		if idx > 0 {
			builder.WriteString(",")
		}
		builder.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			item.ID,
			item.PipelineID,
			string(item.ExecutorType),
			item.ExecutorParamName,
			item.ParamKey,
			string(item.ParamType),
			boolToInt(item.SingleSelect),
			boolToInt(item.Required),
			item.DefaultValue,
			item.Description,
			boolToInt(item.Visible),
			boolToInt(item.Editable),
			string(item.SourceFrom),
			string(item.Status),
			item.RawMeta,
			item.SortNo,
			item.CreatedAt.UTC().UnixNano(),
			item.UpdatedAt.UTC().UnixNano(),
		)
	}

	builder.WriteString(` ON DUPLICATE KEY UPDATE
param_key = CASE WHEN TRIM(param_key) = '' THEN VALUES(param_key) ELSE param_key END,
param_type = VALUES(param_type),
single_select = VALUES(single_select),
required = VALUES(required),
default_value = VALUES(default_value),
description = VALUES(description),
visible = VALUES(visible),
editable = VALUES(editable),
source_from = VALUES(source_from),
status = VALUES(status),
raw_meta = VALUES(raw_meta),
sort_no = VALUES(sort_no),
updated_at = VALUES(updated_at)`)

	_, err := tx.ExecContext(ctx, builder.String(), args...)
	return err
}

// mysqlExistingParamKeys 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) mysqlExistingParamKeys(ctx context.Context, items []domain.ExecutorParamDef) (map[string]struct{}, error) {
	pipelineIDs := make([]string, 0, len(items))
	seenPipelineIDs := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, ok := seenPipelineIDs[item.PipelineID]; ok {
			continue
		}
		seenPipelineIDs[item.PipelineID] = struct{}{}
		pipelineIDs = append(pipelineIDs, item.PipelineID)
	}

	result := make(map[string]struct{}, len(items))
	const chunkSize = 200
	for start := 0; start < len(pipelineIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(pipelineIDs) {
			end = len(pipelineIDs)
		}

		placeholders := strings.TrimRight(strings.Repeat("?,", end-start), ",")
		query := fmt.Sprintf(`SELECT pipeline_id, executor_type, executor_param_name
FROM executor_param_def
WHERE executor_type = ? AND pipeline_id IN (%s)`, placeholders)

		args := make([]any, 0, end-start+1)
		args = append(args, string(domain.ExecutorTypeJenkins))
		for _, pipelineID := range pipelineIDs[start:end] {
			args = append(args, pipelineID)
		}

		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var (
				pipelineID        string
				executorType      string
				executorParamName string
			)
			if err := rows.Scan(&pipelineID, &executorType, &executorParamName); err != nil {
				_ = rows.Close()
				return nil, err
			}
			result[executorParamUniqueKey(pipelineID, domain.ExecutorType(executorType), executorParamName)] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}

	return result, nil
}

// executorParamUniqueKey 封装当前模块的业务处理逻辑。
func executorParamUniqueKey(pipelineID string, executorType domain.ExecutorType, executorParamName string) string {
	return pipelineID + "\x00" + string(executorType) + "\x00" + executorParamName
}

// MarkMissingInactive 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) MarkMissingInactive(
	ctx context.Context,
	executorType domain.ExecutorType,
	keepIDs []string,
	updatedAt time.Time,
) (int, error) {
	if !executorType.Valid() {
		return 0, fmt.Errorf("invalid executor type: %s", executorType)
	}

	query := `UPDATE executor_param_def
SET status = ?, updated_at = ?
WHERE executor_type = ? AND source_from = ? AND status <> ?`
	args := []any{
		string(domain.StatusInactive),
		updatedAt.UTC().UnixNano(),
		string(executorType),
		string(domain.SourceFromSyncJenkins),
		string(domain.StatusInactive),
	}

	keep := make([]string, 0, len(keepIDs))
	for _, id := range keepIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		keep = append(keep, id)
	}
	if len(keep) > 0 {
		query += " AND id NOT IN (" + strings.TrimRight(strings.Repeat("?,", len(keep)), ",") + ")"
		for _, id := range keep {
			args = append(args, id)
		}
	}

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// ListByPipeline 查询并返回列表数据。
func (r *ExecutorParamRepository) ListByPipeline(ctx context.Context, filter domain.ListFilter) ([]domain.ExecutorParamDef, int64, error) {
	where := []string{"pipeline_id = ?"}
	args := []any{filter.PipelineID}

	if filter.ExecutorType != "" {
		where = append(where, "executor_type = ?")
		args = append(args, string(filter.ExecutorType))
	}
	if filter.Visible != nil {
		where = append(where, "visible = ?")
		args = append(args, boolToInt(*filter.Visible))
	}
	if filter.Editable != nil {
		where = append(where, "editable = ?")
		args = append(args, boolToInt(*filter.Editable))
	}
	if filter.ParamKey != "" {
		where = append(where, "param_key = ?")
		args = append(args, filter.ParamKey)
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}

	countQuery := "SELECT COUNT(1) FROM executor_param_def WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := `
SELECT id, pipeline_id, executor_type, executor_param_name, param_key, param_type, single_select, required, default_value, description, visible, editable, source_from, status, raw_meta, sort_no, created_at, updated_at
	FROM executor_param_def
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY sort_no ASC, created_at ASC LIMIT ? OFFSET ?;`

	offset := (filter.Page - 1) * filter.PageSize
	rows, err := r.db.QueryContext(ctx, listQuery, append(args, filter.PageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]domain.ExecutorParamDef, 0)
	for rows.Next() {
		item, scanErr := scanExecutorParam(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListByApplications 查询并返回列表数据。
func (r *ExecutorParamRepository) ListByApplications(
	ctx context.Context,
	filter domain.ApplicationListFilter,
) ([]domain.ExecutorParamDef, int64, error) {
	where := []string{
		"pb.provider = ?",
		"pb.status = ?",
		"epd.executor_type = ?",
	}
	args := []any{
		string(pipelinedomain.ProviderJenkins),
		string(pipelinedomain.StatusActive),
		string(domain.ExecutorTypeJenkins),
	}

	applicationIDs := make([]string, 0, len(filter.ApplicationIDs))
	for _, item := range filter.ApplicationIDs {
		applicationID := strings.TrimSpace(item)
		if applicationID == "" {
			continue
		}
		applicationIDs = append(applicationIDs, applicationID)
	}
	if len(applicationIDs) > 0 {
		where = append(where, "pb.application_id IN ("+strings.TrimRight(strings.Repeat("?,", len(applicationIDs)), ",")+")")
		for _, applicationID := range applicationIDs {
			args = append(args, applicationID)
		}
	}
	if filter.BindingType != "" {
		where = append(where, "pb.binding_type = ?")
		args = append(args, string(filter.BindingType))
	}
	if filter.Visible != nil {
		where = append(where, "epd.visible = ?")
		args = append(args, boolToInt(*filter.Visible))
	}
	if filter.Editable != nil {
		where = append(where, "epd.editable = ?")
		args = append(args, boolToInt(*filter.Editable))
	}
	if filter.Status != "" {
		where = append(where, "epd.status = ?")
		args = append(args, string(filter.Status))
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		pattern := "%" + keyword + "%"
		where = append(
			where,
			`(
COALESCE(NULLIF(a.name, ''), NULLIF(pb.application_name, ''), pb.application_id) LIKE ?
OR COALESCE(NULLIF(a.app_key, ''), '') LIKE ?
OR epd.param_key LIKE ?
)`,
		)
		args = append(args, pattern, pattern, pattern)
	}

	baseQuery := `
FROM executor_param_def epd
INNER JOIN pipeline_bindings pb ON pb.pipeline_id = epd.pipeline_id
INNER JOIN applications a ON a.id = pb.application_id
WHERE ` + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) "+baseQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := `
SELECT
	pb.application_id,
	COALESCE(NULLIF(a.name, ''), NULLIF(pb.application_name, ''), pb.application_id) AS application_name,
	COALESCE(NULLIF(a.app_key, ''), '') AS application_key,
	pb.binding_type,
	COALESCE(NULLIF(pb.name, ''), NULLIF(pb.pipeline_id, ''), epd.pipeline_id) AS pipeline_name,
	epd.id,
	epd.pipeline_id,
	epd.executor_type,
	epd.executor_param_name,
	epd.param_key,
	epd.param_type,
	epd.single_select,
	epd.required,
	epd.default_value,
	epd.description,
	epd.visible,
	epd.editable,
	epd.source_from,
	epd.status,
	epd.raw_meta,
	epd.sort_no,
	epd.created_at,
	epd.updated_at
` + baseQuery + `
ORDER BY application_name ASC, application_key ASC, pb.binding_type ASC, pipeline_name ASC, epd.sort_no ASC, epd.created_at ASC
LIMIT ? OFFSET ?;`

	offset := (filter.Page - 1) * filter.PageSize
	rows, err := r.db.QueryContext(ctx, listQuery, append(args, filter.PageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]domain.ExecutorParamDef, 0)
	for rows.Next() {
		item, scanErr := scanExecutorParamWithApplication(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetByID 查询并返回指定资源数据。
func (r *ExecutorParamRepository) GetByID(ctx context.Context, id string) (domain.ExecutorParamDef, error) {
	const q = `
SELECT id, pipeline_id, executor_type, executor_param_name, param_key, param_type, single_select, required, default_value, description, visible, editable, source_from, status, raw_meta, sort_no, created_at, updated_at
FROM executor_param_def
WHERE id = ?;`

	row := r.db.QueryRowContext(ctx, q, id)
	item, err := scanExecutorParam(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ExecutorParamDef{}, domain.ErrNotFound
		}
		return domain.ExecutorParamDef{}, err
	}
	return item, nil
}

// UpdateParamKey 更新业务资源并返回处理结果。
func (r *ExecutorParamRepository) UpdateParamKey(ctx context.Context, id string, paramKey string, updatedAt time.Time) (domain.ExecutorParamDef, error) {
	const q = `
UPDATE executor_param_def
SET param_key = ?, updated_at = ?
WHERE id = ?;`

	res, err := r.db.ExecContext(ctx, q, paramKey, updatedAt.UTC().UnixNano(), id)
	if err != nil {
		return domain.ExecutorParamDef{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.ExecutorParamDef{}, err
	}
	if affected == 0 {
		return domain.ExecutorParamDef{}, domain.ErrNotFound
	}
	return r.GetByID(ctx, id)
}

// CountByParamKey 封装当前模块的业务处理逻辑。
func (r *ExecutorParamRepository) CountByParamKey(ctx context.Context, paramKey string) (int64, error) {
	const q = `SELECT COUNT(1) FROM executor_param_def WHERE param_key = ?;`
	var total int64
	if err := r.db.QueryRowContext(ctx, q, paramKey).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// scanExecutorParam 封装当前模块的业务处理逻辑。
func scanExecutorParam(s scanner) (domain.ExecutorParamDef, error) {
	var (
		item         domain.ExecutorParamDef
		executorType string
		paramType    string
		singleSelect int
		required     int
		visible      int
		editable     int
		sourceFrom   string
		status       string
		rawMeta      sql.NullString
		createdAt    int64
		updatedAt    int64
	)

	if err := s.Scan(
		&item.ID,
		&item.PipelineID,
		&executorType,
		&item.ExecutorParamName,
		&item.ParamKey,
		&paramType,
		&singleSelect,
		&required,
		&item.DefaultValue,
		&item.Description,
		&visible,
		&editable,
		&sourceFrom,
		&status,
		&rawMeta,
		&item.SortNo,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.ExecutorParamDef{}, err
	}

	item.ExecutorType = domain.ExecutorType(executorType)
	item.ParamType = domain.ParamType(paramType)
	item.SingleSelect = singleSelect > 0
	item.Required = required > 0
	item.Visible = visible > 0
	item.Editable = editable > 0
	item.SourceFrom = domain.SourceFrom(sourceFrom)
	item.Status = domain.Status(status)
	if item.Status == "" {
		item.Status = domain.StatusActive
	}
	item.RawMeta = rawMeta.String
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

// scanExecutorParamWithApplication 封装当前模块的业务处理逻辑。
func scanExecutorParamWithApplication(s scanner) (domain.ExecutorParamDef, error) {
	var (
		item         domain.ExecutorParamDef
		executorType string
		paramType    string
		singleSelect int
		required     int
		visible      int
		editable     int
		sourceFrom   string
		status       string
		rawMeta      sql.NullString
		createdAt    int64
		updatedAt    int64
	)

	if err := s.Scan(
		&item.ApplicationID,
		&item.ApplicationName,
		&item.ApplicationKey,
		&item.BindingType,
		&item.PipelineName,
		&item.ID,
		&item.PipelineID,
		&executorType,
		&item.ExecutorParamName,
		&item.ParamKey,
		&paramType,
		&singleSelect,
		&required,
		&item.DefaultValue,
		&item.Description,
		&visible,
		&editable,
		&sourceFrom,
		&status,
		&rawMeta,
		&item.SortNo,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.ExecutorParamDef{}, err
	}

	item.ExecutorType = domain.ExecutorType(executorType)
	item.ParamType = domain.ParamType(paramType)
	item.SingleSelect = singleSelect > 0
	item.Required = required > 0
	item.Visible = visible > 0
	item.Editable = editable > 0
	item.SourceFrom = domain.SourceFrom(sourceFrom)
	item.Status = domain.Status(status)
	if item.Status == "" {
		item.Status = domain.StatusActive
	}
	item.RawMeta = rawMeta.String
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}
