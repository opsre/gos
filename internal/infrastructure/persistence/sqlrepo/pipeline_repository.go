package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/pipeline"
)

type PipelineRepository struct {
	db       *sql.DB
	dbDriver string
}

// NewPipelineRepository 创建并返回对应组件实例。
func NewPipelineRepository(db *sql.DB, dbDriver string) *PipelineRepository {
	return &PipelineRepository{
		db:       db,
		dbDriver: strings.ToLower(strings.TrimSpace(dbDriver)),
	}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (r *PipelineRepository) InitSchema(ctx context.Context) error {
	statements, err := pipelineSchemaStatements(r.dbDriver)
	if err != nil {
		return err
	}
	for _, stmt := range statements {
		if _, execErr := r.db.ExecContext(ctx, stmt); execErr != nil {
			return execErr
		}
	}
	return r.migratePipelineBindingSchema(ctx)
}

// pipelineSchemaStatements 封装当前模块的业务处理逻辑。
func pipelineSchemaStatements(dbDriver string) ([]string, error) {
	switch dbDriver {
	case "mysql":
		return []string{
			`CREATE TABLE IF NOT EXISTS pipelines (
	id VARCHAR(64) PRIMARY KEY,
	provider VARCHAR(32) NOT NULL,
	job_full_name VARCHAR(255) NOT NULL,
	job_name VARCHAR(255) NOT NULL,
	job_url TEXT NOT NULL,
	description TEXT NOT NULL,
	credential_ref VARCHAR(255) NOT NULL,
	default_branch VARCHAR(255) NOT NULL,
	status VARCHAR(32) NOT NULL,
	last_verified_at BIGINT NULL,
	last_synced_at BIGINT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_pipeline_provider_full_name (provider, job_full_name),
	KEY idx_pipeline_status_updated_at (status, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS pipeline_bindings (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(128) NOT NULL DEFAULT '',
	application_id VARCHAR(64) NOT NULL,
	application_name VARCHAR(128) NOT NULL DEFAULT '',
	binding_type VARCHAR(32) NOT NULL,
	provider VARCHAR(32) NOT NULL,
	pipeline_id VARCHAR(64) NOT NULL DEFAULT '',
	external_ref VARCHAR(255) NOT NULL DEFAULT '',
	trigger_mode VARCHAR(32) NOT NULL,
	status VARCHAR(32) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_binding_app_pipeline (application_id, pipeline_id),
	UNIQUE KEY uq_binding_app_type (application_id, binding_type),
	KEY idx_binding_app_created_at (application_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}, nil
	case "sqlite":
		return []string{
			`CREATE TABLE IF NOT EXISTS pipelines (
	id TEXT PRIMARY KEY,
	provider TEXT NOT NULL,
	job_full_name TEXT NOT NULL,
	job_name TEXT NOT NULL,
	job_url TEXT NOT NULL,
	description TEXT NOT NULL,
	credential_ref TEXT NOT NULL,
	default_branch TEXT NOT NULL,
	status TEXT NOT NULL,
	last_verified_at INTEGER NULL,
	last_synced_at INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE(provider, job_full_name)
);`,
			`CREATE INDEX IF NOT EXISTS idx_pipeline_status_updated_at ON pipelines (status, updated_at);`,
			`CREATE TABLE IF NOT EXISTS pipeline_bindings (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL DEFAULT '',
	application_id TEXT NOT NULL,
	application_name TEXT NOT NULL DEFAULT '',
	binding_type TEXT NOT NULL,
	provider TEXT NOT NULL,
	pipeline_id TEXT NOT NULL DEFAULT '',
	external_ref TEXT NOT NULL DEFAULT '',
	trigger_mode TEXT NOT NULL,
	status TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE(application_id, pipeline_id)
);`,
			`CREATE INDEX IF NOT EXISTS idx_binding_app_created_at ON pipeline_bindings (application_id, created_at);`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uq_binding_app_type ON pipeline_bindings (application_id, binding_type);`,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported db driver: %s", dbDriver)
	}
}

// migratePipelineBindingSchema 封装当前模块的业务处理逻辑。
func (r *PipelineRepository) migratePipelineBindingSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
		return r.migratePipelineBindingSchemaMySQL(ctx)
	case "sqlite":
		return r.migratePipelineBindingSchemaSQLite(ctx)
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

// migratePipelineBindingSchemaMySQL 封装当前模块的业务处理逻辑。
func (r *PipelineRepository) migratePipelineBindingSchemaMySQL(ctx context.Context) error {
	type columnDef struct {
		name string
		ddl  string
	}
	columns := []columnDef{
		{
			name: "name",
			ddl:  `ALTER TABLE pipeline_bindings ADD COLUMN name VARCHAR(128) NOT NULL DEFAULT '' AFTER id;`,
		},
		{
			name: "binding_type",
			ddl:  `ALTER TABLE pipeline_bindings ADD COLUMN binding_type VARCHAR(32) NOT NULL DEFAULT 'ci' AFTER application_id;`,
		},
		{
			name: "application_name",
			ddl:  `ALTER TABLE pipeline_bindings ADD COLUMN application_name VARCHAR(128) NOT NULL DEFAULT '' AFTER application_id;`,
		},
		{
			name: "provider",
			ddl:  `ALTER TABLE pipeline_bindings ADD COLUMN provider VARCHAR(32) NOT NULL DEFAULT 'jenkins' AFTER binding_type;`,
		},
		{
			name: "external_ref",
			ddl:  `ALTER TABLE pipeline_bindings ADD COLUMN external_ref VARCHAR(255) NOT NULL DEFAULT '' AFTER pipeline_id;`,
		},
	}

	for _, column := range columns {
		exists, err := r.mysqlColumnExists(ctx, "pipeline_bindings", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := r.db.ExecContext(ctx, column.ddl); err != nil {
			return err
		}
	}

	if _, err := r.db.ExecContext(ctx, `
UPDATE pipeline_bindings pb
LEFT JOIN applications a ON a.id = pb.application_id
SET pb.application_name = COALESCE(NULLIF(a.name, ''), pb.application_id)
WHERE pb.application_name = '';
`); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE pipeline_bindings pb
LEFT JOIN pipelines p ON p.id = pb.pipeline_id
SET pb.name = CASE
	WHEN pb.provider = 'argocd' THEN COALESCE(NULLIF(pb.external_ref, ''), pb.binding_type)
	ELSE COALESCE(NULLIF(p.job_name, ''), NULLIF(p.job_full_name, ''), NULLIF(pb.pipeline_id, ''), pb.binding_type)
END
WHERE pb.name = '';
`); err != nil {
		return err
	}

	indexExists, err := r.mysqlIndexExists(ctx, "pipeline_bindings", "uq_binding_app_type")
	if err != nil {
		return err
	}
	if !indexExists {
		if _, err := r.db.ExecContext(
			ctx,
			`ALTER TABLE pipeline_bindings ADD UNIQUE KEY uq_binding_app_type (application_id, binding_type);`,
		); err != nil {
			return err
		}
	}

	return nil
}

// migratePipelineBindingSchemaSQLite 封装当前模块的业务处理逻辑。
func (r *PipelineRepository) migratePipelineBindingSchemaSQLite(ctx context.Context) error {
	columns, err := r.sqliteTableColumns(ctx, "pipeline_bindings")
	if err != nil {
		return err
	}

	type columnDef struct {
		name string
		ddl  string
	}
	additions := []columnDef{
		{
			name: "name",
			ddl:  `ALTER TABLE pipeline_bindings ADD COLUMN name TEXT NOT NULL DEFAULT '';`,
		},
		{
			name: "binding_type",
			ddl:  `ALTER TABLE pipeline_bindings ADD COLUMN binding_type TEXT NOT NULL DEFAULT 'ci';`,
		},
		{
			name: "application_name",
			ddl:  `ALTER TABLE pipeline_bindings ADD COLUMN application_name TEXT NOT NULL DEFAULT '';`,
		},
		{
			name: "provider",
			ddl:  `ALTER TABLE pipeline_bindings ADD COLUMN provider TEXT NOT NULL DEFAULT 'jenkins';`,
		},
		{
			name: "external_ref",
			ddl:  `ALTER TABLE pipeline_bindings ADD COLUMN external_ref TEXT NOT NULL DEFAULT '';`,
		},
	}

	for _, column := range additions {
		if _, ok := columns[column.name]; ok {
			continue
		}
		if _, err := r.db.ExecContext(ctx, column.ddl); err != nil {
			return err
		}
	}

	if _, err := r.db.ExecContext(ctx, `
UPDATE pipeline_bindings
SET application_name = COALESCE(
	(SELECT name FROM applications WHERE applications.id = pipeline_bindings.application_id),
	application_id
)
WHERE application_name = '';
`); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE pipeline_bindings
SET name = CASE
	WHEN provider = 'argocd' THEN COALESCE(NULLIF(external_ref, ''), binding_type)
	ELSE COALESCE(
		(SELECT job_name FROM pipelines WHERE pipelines.id = pipeline_bindings.pipeline_id),
		(SELECT job_full_name FROM pipelines WHERE pipelines.id = pipeline_bindings.pipeline_id),
		NULLIF(pipeline_id, ''),
		binding_type
	)
END
WHERE name = '';
`); err != nil {
		return err
	}

	if _, err := r.db.ExecContext(
		ctx,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_binding_app_type ON pipeline_bindings (application_id, binding_type);`,
	); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(
		ctx,
		`CREATE INDEX IF NOT EXISTS idx_binding_app_created_at ON pipeline_bindings (application_id, created_at);`,
	); err != nil {
		return err
	}
	return nil
}

// mysqlColumnExists 封装当前模块的业务处理逻辑。
func (r *PipelineRepository) mysqlColumnExists(ctx context.Context, table, column string) (bool, error) {
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

// mysqlIndexExists 封装当前模块的业务处理逻辑。
func (r *PipelineRepository) mysqlIndexExists(ctx context.Context, table, index string) (bool, error) {
	const q = `
SELECT COUNT(1)
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?;`

	var count int
	if err := r.db.QueryRowContext(ctx, q, table, index).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// sqliteTableColumns 封装当前模块的业务处理逻辑。
func (r *PipelineRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
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

// UpsertPipelines 封装当前模块的业务处理逻辑。
func (r *PipelineRepository) UpsertPipelines(ctx context.Context, items []domain.Pipeline) (int, int, error) {
	const (
		updateByKey = `UPDATE pipelines
SET job_name = ?, job_url = ?, description = ?, credential_ref = ?, default_branch = ?, status = ?, last_synced_at = ?, updated_at = ?
WHERE provider = ? AND job_full_name = ?;`
		insert = `INSERT INTO pipelines (
	id, provider, job_full_name, job_name, job_url, description, credential_ref, default_branch, status, last_verified_at, last_synced_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	)

	created := 0
	updated := 0

	for _, item := range items {
		lastSynced := item.LastSyncedAt.UTC().UnixNano()
		updatedAt := item.UpdatedAt.UTC().UnixNano()

		res, err := r.db.ExecContext(
			ctx,
			updateByKey,
			item.JobName,
			item.JobURL,
			item.Description,
			item.CredentialRef,
			item.DefaultBranch,
			string(item.Status),
			lastSynced,
			updatedAt,
			string(item.Provider),
			item.JobFullName,
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

		var lastVerified any
		if item.LastVerifiedAt != nil {
			lastVerified = item.LastVerifiedAt.UTC().UnixNano()
		}

		_, err = r.db.ExecContext(
			ctx,
			insert,
			item.ID,
			string(item.Provider),
			item.JobFullName,
			item.JobName,
			item.JobURL,
			item.Description,
			item.CredentialRef,
			item.DefaultBranch,
			string(item.Status),
			lastVerified,
			lastSynced,
			item.CreatedAt.UTC().UnixNano(),
			updatedAt,
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
	return created, updated, nil
}

// MarkMissingPipelinesInactive 封装当前模块的业务处理逻辑。
func (r *PipelineRepository) MarkMissingPipelinesInactive(
	ctx context.Context,
	provider domain.Provider,
	keepIDs []string,
	updatedAt time.Time,
) (int, error) {
	if !provider.Valid() {
		return 0, fmt.Errorf("invalid pipeline provider: %s", provider)
	}

	query := `UPDATE pipelines SET status = ?, updated_at = ? WHERE provider = ? AND status <> ?`
	args := []any{
		string(domain.StatusInactive),
		updatedAt.UTC().UnixNano(),
		string(provider),
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

// ListPipelines 查询并返回列表数据。
func (r *PipelineRepository) ListPipelines(ctx context.Context, filter domain.PipelineListFilter) ([]domain.Pipeline, int64, error) {
	where := make([]string, 0, 3)
	args := make([]any, 0, 3)

	if filter.Name != "" {
		where = append(where, "job_name LIKE ?")
		args = append(args, "%"+filter.Name+"%")
	}
	if filter.Provider != "" {
		where = append(where, "provider = ?")
		args = append(args, string(filter.Provider))
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}

	var countBuilder strings.Builder
	countBuilder.WriteString(`SELECT COUNT(1) FROM pipelines`)
	if len(where) > 0 {
		countBuilder.WriteString(" WHERE ")
		countBuilder.WriteString(strings.Join(where, " AND "))
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, countBuilder.String(), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	var listBuilder strings.Builder
	listBuilder.WriteString(`
SELECT id, provider, job_full_name, job_name, job_url, description, credential_ref, default_branch, status, last_verified_at, last_synced_at, created_at, updated_at
FROM pipelines`)
	if len(where) > 0 {
		listBuilder.WriteString(" WHERE ")
		listBuilder.WriteString(strings.Join(where, " AND "))
	}
	listBuilder.WriteString(" ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?;")

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, listBuilder.String(), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]domain.Pipeline, 0)
	for rows.Next() {
		item, scanErr := scanPipeline(rows)
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

// GetPipelineByID 查询并返回指定资源数据。
func (r *PipelineRepository) GetPipelineByID(ctx context.Context, id string) (domain.Pipeline, error) {
	const q = `
SELECT id, provider, job_full_name, job_name, job_url, description, credential_ref, default_branch, status, last_verified_at, last_synced_at, created_at, updated_at
FROM pipelines
WHERE id = ?;`
	row := r.db.QueryRowContext(ctx, q, id)
	item, err := scanPipeline(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Pipeline{}, domain.ErrPipelineNotFound
		}
		return domain.Pipeline{}, err
	}
	return item, nil
}

// MarkPipelineVerified 封装当前模块的业务处理逻辑。
func (r *PipelineRepository) MarkPipelineVerified(ctx context.Context, id string, verifiedAt time.Time, updatedAt time.Time) (domain.Pipeline, error) {
	const q = `
UPDATE pipelines
SET last_verified_at = ?, updated_at = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, verifiedAt.UTC().UnixNano(), updatedAt.UTC().UnixNano(), id)
	if err != nil {
		return domain.Pipeline{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Pipeline{}, err
	}
	if affected == 0 {
		return domain.Pipeline{}, domain.ErrPipelineNotFound
	}
	return r.GetPipelineByID(ctx, id)
}

// CreateBinding 创建业务资源并返回处理结果。
func (r *PipelineRepository) CreateBinding(ctx context.Context, binding domain.PipelineBinding) error {
	const q = `
INSERT INTO pipeline_bindings (
	id, name, application_id, application_name, binding_type, provider, pipeline_id, external_ref, trigger_mode, status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(
		ctx,
		q,
		binding.ID,
		binding.Name,
		binding.ApplicationID,
		binding.ApplicationName,
		string(binding.BindingType),
		string(binding.Provider),
		binding.PipelineID,
		binding.ExternalRef,
		string(binding.TriggerMode),
		string(binding.Status),
		binding.CreatedAt.UTC().UnixNano(),
		binding.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.ErrBindingDuplicated
		}
		return err
	}
	return nil
}

// ListBindingsByApplication 查询并返回列表数据。
func (r *PipelineRepository) ListBindingsByApplication(ctx context.Context, filter domain.BindingListFilter) ([]domain.PipelineBinding, int64, error) {
	where := []string{"application_id = ?"}
	args := []any{filter.ApplicationID}

	if filter.BindingType != "" {
		where = append(where, "binding_type = ?")
		args = append(args, string(filter.BindingType))
	}
	if filter.Provider != "" {
		where = append(where, "provider = ?")
		args = append(args, string(filter.Provider))
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}

	countQuery := "SELECT COUNT(1) FROM pipeline_bindings WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := `
SELECT id, name, application_id, application_name, binding_type, provider, pipeline_id, external_ref, trigger_mode, status, created_at, updated_at
FROM pipeline_bindings
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY created_at DESC LIMIT ? OFFSET ?;`
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, listQuery, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]domain.PipelineBinding, 0)
	for rows.Next() {
		item, scanErr := scanPipelineBinding(rows)
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

// GetBindingByID 查询并返回指定资源数据。
func (r *PipelineRepository) GetBindingByID(ctx context.Context, id string) (domain.PipelineBinding, error) {
	const q = `
SELECT id, name, application_id, application_name, binding_type, provider, pipeline_id, external_ref, trigger_mode, status, created_at, updated_at
FROM pipeline_bindings
WHERE id = ?;`
	row := r.db.QueryRowContext(ctx, q, id)
	item, err := scanPipelineBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.PipelineBinding{}, domain.ErrBindingNotFound
		}
		return domain.PipelineBinding{}, err
	}
	return item, nil
}

// UpdateBinding 更新业务资源并返回处理结果。
func (r *PipelineRepository) UpdateBinding(ctx context.Context, id string, input domain.BindingUpdateInput, updatedAt time.Time) (domain.PipelineBinding, error) {
	const q = `
UPDATE pipeline_bindings
SET name = ?, provider = ?, pipeline_id = ?, external_ref = ?, trigger_mode = ?, status = ?, updated_at = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(
		ctx,
		q,
		input.Name,
		string(input.Provider),
		input.PipelineID,
		input.ExternalRef,
		string(input.TriggerMode),
		string(input.Status),
		updatedAt.UTC().UnixNano(),
		id,
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.PipelineBinding{}, domain.ErrBindingDuplicated
		}
		return domain.PipelineBinding{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.PipelineBinding{}, err
	}
	if affected == 0 {
		return domain.PipelineBinding{}, domain.ErrBindingNotFound
	}
	return r.GetBindingByID(ctx, id)
}

// DeleteBinding 删除业务资源并返回处理结果。
func (r *PipelineRepository) DeleteBinding(ctx context.Context, id string) error {
	const q = `DELETE FROM pipeline_bindings WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrBindingNotFound
	}
	return nil
}

// scanPipeline 封装当前模块的业务处理逻辑。
func scanPipeline(s scanner) (domain.Pipeline, error) {
	var (
		item           domain.Pipeline
		providerRaw    string
		statusRaw      string
		lastVerifiedAt sql.NullInt64
		lastSyncedAt   int64
		createdAt      int64
		updatedAt      int64
	)
	if err := s.Scan(
		&item.ID,
		&providerRaw,
		&item.JobFullName,
		&item.JobName,
		&item.JobURL,
		&item.Description,
		&item.CredentialRef,
		&item.DefaultBranch,
		&statusRaw,
		&lastVerifiedAt,
		&lastSyncedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Pipeline{}, err
	}
	item.Provider = domain.Provider(providerRaw)
	item.Status = domain.Status(statusRaw)
	if lastVerifiedAt.Valid {
		t := time.Unix(0, lastVerifiedAt.Int64).UTC()
		item.LastVerifiedAt = &t
	}
	item.LastSyncedAt = time.Unix(0, lastSyncedAt).UTC()
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

// scanPipelineBinding 封装当前模块的业务处理逻辑。
func scanPipelineBinding(s scanner) (domain.PipelineBinding, error) {
	var (
		item           domain.PipelineBinding
		bindingTypeRaw string
		providerRaw    string
		triggerModeRaw string
		statusRaw      string
		createdAt      int64
		updatedAt      int64
	)
	if err := s.Scan(
		&item.ID,
		&item.Name,
		&item.ApplicationID,
		&item.ApplicationName,
		&bindingTypeRaw,
		&providerRaw,
		&item.PipelineID,
		&item.ExternalRef,
		&triggerModeRaw,
		&statusRaw,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.PipelineBinding{}, err
	}
	item.BindingType = domain.BindingType(bindingTypeRaw)
	if item.BindingType == "" {
		item.BindingType = domain.BindingTypeCI
	}
	item.Provider = domain.Provider(providerRaw)
	if item.Provider == "" {
		item.Provider = domain.ProviderJenkins
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		if ref := strings.TrimSpace(item.ExternalRef); ref != "" {
			item.Name = ref
		} else if pid := strings.TrimSpace(item.PipelineID); pid != "" {
			item.Name = pid
		} else {
			item.Name = string(item.BindingType)
		}
	}
	item.ApplicationName = strings.TrimSpace(item.ApplicationName)
	if item.ApplicationName == "" {
		item.ApplicationName = item.ApplicationID
	}
	item.TriggerMode = domain.TriggerMode(triggerModeRaw)
	item.Status = domain.Status(statusRaw)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}
