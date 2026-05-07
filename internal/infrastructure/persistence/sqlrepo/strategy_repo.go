package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gos/internal/domain/strategy"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type StrategyRepository struct {
	db       *sql.DB
	dbDriver string
}

func NewStrategyRepository(db *sql.DB, dbDriver string) *StrategyRepository {
	return &StrategyRepository{db: db, dbDriver: strings.ToLower(strings.TrimSpace(dbDriver))}
}

func (r *StrategyRepository) InitSchema(ctx context.Context) error {
	statements, err := strategySchemaStatements(r.dbDriver)
	if err != nil {
		return err
	}
	for _, stmt := range statements {
		if _, err = r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return r.migrateSchema(ctx)
}

func strategySchemaStatements(dbDriver string) ([]string, error) {
	switch dbDriver {
	case "mysql":
		return []string{
			`CREATE TABLE IF NOT EXISTS release_strategy_templates (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(128) NOT NULL,
	strategy_engine VARCHAR(32) NOT NULL DEFAULT 'k8s_native',
	strategy_type VARCHAR(32) NOT NULL DEFAULT 'rolling_update',
	strategy_config TEXT NOT NULL,
	description TEXT NOT NULL,
	status VARCHAR(32) NOT NULL DEFAULT 'active',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_release_strategy_template_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS application_env_runtime_bindings (
	id VARCHAR(64) PRIMARY KEY,
	application_id VARCHAR(64) NOT NULL,
	env_code VARCHAR(64) NOT NULL,
	k8s_cluster_ref_id VARCHAR(64) NOT NULL,
	namespace VARCHAR(128) NOT NULL DEFAULT 'default',
	workload_name VARCHAR(128) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_app_env_runtime (application_id, env_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS application_env_strategy_bindings (
	id VARCHAR(64) PRIMARY KEY,
	application_id VARCHAR(64) NOT NULL,
	env_code VARCHAR(64) NOT NULL,
	strategy_template_id VARCHAR(64) NOT NULL,
	overrides_config TEXT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_app_env_strategy (application_id, env_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}, nil
	case "sqlite":
		return []string{
			`CREATE TABLE IF NOT EXISTS release_strategy_templates (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	strategy_engine TEXT NOT NULL DEFAULT 'k8s_native',
	strategy_type TEXT NOT NULL DEFAULT 'rolling_update',
	strategy_config TEXT NOT NULL,
	description TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE TABLE IF NOT EXISTS application_env_runtime_bindings (
	id TEXT PRIMARY KEY,
	application_id TEXT NOT NULL,
	env_code TEXT NOT NULL,
	k8s_cluster_ref_id TEXT NOT NULL,
	namespace TEXT NOT NULL DEFAULT 'default',
	workload_name TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE (application_id, env_code)
);`,
			`CREATE TABLE IF NOT EXISTS application_env_strategy_bindings (
	id TEXT PRIMARY KEY,
	application_id TEXT NOT NULL,
	env_code TEXT NOT NULL,
	strategy_template_id TEXT NOT NULL,
	overrides_config TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE (application_id, env_code)
);`,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported db driver: %s", dbDriver)
	}
}

func (r *StrategyRepository) migrateSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
	case "sqlite":
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	return nil
}

func (r *StrategyRepository) CreateTemplate(ctx context.Context, item strategy.ReleaseStrategyTemplate) error {
	const q = `
INSERT INTO release_strategy_templates (id, name, strategy_engine, strategy_type, strategy_config, description, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(ctx, q,
		item.ID,
		item.Name,
		string(item.StrategyEngine),
		string(item.StrategyType),
		item.StrategyConfig,
		item.Description,
		string(item.Status),
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isStrategyDuplicateNameError(r.dbDriver, err) {
			return strategy.ErrTemplateNameDuplicated
		}
		return err
	}
	return nil
}

func (r *StrategyRepository) GetTemplateByID(ctx context.Context, id string) (strategy.ReleaseStrategyTemplate, error) {
	const q = `
SELECT id, name, strategy_engine, strategy_type, strategy_config, description, status, created_at, updated_at
FROM release_strategy_templates
WHERE id = ?;`
	row := r.db.QueryRowContext(ctx, q, id)
	item, err := scanReleaseStrategyTemplate(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return strategy.ReleaseStrategyTemplate{}, strategy.ErrTemplateNotFound
		}
		return strategy.ReleaseStrategyTemplate{}, err
	}
	return item, nil
}

func (r *StrategyRepository) ListTemplates(ctx context.Context, filter strategy.TemplateListFilter) ([]strategy.ReleaseStrategyTemplate, int64, error) {
	args := make([]any, 0, 4)
	where := make([]string, 0, 4)
	if filter.StrategyEngine != "" {
		where = append(where, "strategy_engine = ?")
		args = append(args, string(filter.StrategyEngine))
	}
	if filter.StrategyType != "" {
		where = append(where, "strategy_type = ?")
		args = append(args, string(filter.StrategyType))
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.Name != "" {
		where = append(where, "name = ?")
		args = append(args, filter.Name)
	}

	countSQL := strings.Builder{}
	countSQL.WriteString("SELECT COUNT(1) FROM release_strategy_templates")
	if len(where) > 0 {
		countSQL.WriteString(" WHERE ")
		countSQL.WriteString(strings.Join(where, " AND "))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL.String(), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := strings.Builder{}
	query.WriteString(`
SELECT id, name, strategy_engine, strategy_type, strategy_config, description, status, created_at, updated_at
FROM release_strategy_templates`)
	if len(where) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(where, " AND "))
	}
	query.WriteString(" ORDER BY created_at DESC LIMIT ? OFFSET ?;")
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := r.db.QueryContext(ctx, query.String(), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]strategy.ReleaseStrategyTemplate, 0)
	for rows.Next() {
		item, err := scanReleaseStrategyTemplate(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *StrategyRepository) UpdateTemplate(ctx context.Context, id string, input strategy.TemplateUpdateInput) (strategy.ReleaseStrategyTemplate, error) {
	const q = `
UPDATE release_strategy_templates
SET name = ?, strategy_engine = ?, strategy_type = ?, strategy_config = ?, description = ?, status = ?, updated_at = ?
WHERE id = ?;`
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, q,
		input.Name,
		string(input.StrategyEngine),
		string(input.StrategyType),
		input.StrategyConfig,
		input.Description,
		string(input.Status),
		now.UTC().UnixNano(),
		id,
	)
	if err != nil {
		if isStrategyDuplicateNameError(r.dbDriver, err) {
			return strategy.ReleaseStrategyTemplate{}, strategy.ErrTemplateNameDuplicated
		}
		return strategy.ReleaseStrategyTemplate{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return strategy.ReleaseStrategyTemplate{}, err
	}
	if affected == 0 {
		return strategy.ReleaseStrategyTemplate{}, strategy.ErrTemplateNotFound
	}
	return r.GetTemplateByID(ctx, id)
}

func (r *StrategyRepository) DeleteTemplate(ctx context.Context, id string) error {
	const countQ = `SELECT COUNT(1) FROM application_env_strategy_bindings WHERE strategy_template_id = ?;`
	var refs int64
	if err := r.db.QueryRowContext(ctx, countQ, id).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return strategy.ErrTemplateInUse
	}

	const q = `DELETE FROM release_strategy_templates WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return strategy.ErrTemplateNotFound
	}
	return nil
}

func (r *StrategyRepository) CreateRuntimeBinding(ctx context.Context, item strategy.ApplicationEnvRuntimeBinding) error {
	const q = `
INSERT INTO application_env_runtime_bindings (id, application_id, env_code, k8s_cluster_ref_id, namespace, workload_name, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(ctx, q,
		item.ID,
		item.ApplicationID,
		item.EnvCode,
		item.K8sClusterRefID,
		item.Namespace,
		item.WorkloadName,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isStrategyBindingDuplicateError(r.dbDriver, err) {
			return strategy.ErrRuntimeBindingDuplicated
		}
		return err
	}
	return nil
}

func (r *StrategyRepository) GetRuntimeBindingByID(ctx context.Context, id string) (strategy.ApplicationEnvRuntimeBinding, error) {
	const q = `
SELECT id, application_id, env_code, k8s_cluster_ref_id, namespace, workload_name, created_at, updated_at
FROM application_env_runtime_bindings
WHERE id = ?;`
	row := r.db.QueryRowContext(ctx, q, id)
	item, err := scanApplicationEnvRuntimeBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return strategy.ApplicationEnvRuntimeBinding{}, strategy.ErrRuntimeBindingNotFound
		}
		return strategy.ApplicationEnvRuntimeBinding{}, err
	}
	return item, nil
}

func (r *StrategyRepository) GetRuntimeBindingByAppEnv(ctx context.Context, applicationID, envCode string) (strategy.ApplicationEnvRuntimeBinding, error) {
	const q = `
SELECT id, application_id, env_code, k8s_cluster_ref_id, namespace, workload_name, created_at, updated_at
FROM application_env_runtime_bindings
WHERE application_id = ? AND env_code = ?;`
	row := r.db.QueryRowContext(ctx, q, applicationID, envCode)
	item, err := scanApplicationEnvRuntimeBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return strategy.ApplicationEnvRuntimeBinding{}, strategy.ErrRuntimeBindingNotFound
		}
		return strategy.ApplicationEnvRuntimeBinding{}, err
	}
	return item, nil
}

func (r *StrategyRepository) ListRuntimeBindings(ctx context.Context, filter strategy.RuntimeBindingListFilter) ([]strategy.ApplicationEnvRuntimeBinding, int64, error) {
	args := make([]any, 0, 2)
	where := make([]string, 0, 2)
	if filter.ApplicationID != "" {
		where = append(where, "application_id = ?")
		args = append(args, filter.ApplicationID)
	}
	if filter.EnvCode != "" {
		where = append(where, "env_code = ?")
		args = append(args, filter.EnvCode)
	}

	countSQL := strings.Builder{}
	countSQL.WriteString("SELECT COUNT(1) FROM application_env_runtime_bindings")
	if len(where) > 0 {
		countSQL.WriteString(" WHERE ")
		countSQL.WriteString(strings.Join(where, " AND "))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL.String(), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := strings.Builder{}
	query.WriteString(`
SELECT id, application_id, env_code, k8s_cluster_ref_id, namespace, workload_name, created_at, updated_at
FROM application_env_runtime_bindings`)
	if len(where) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(where, " AND "))
	}
	query.WriteString(" ORDER BY created_at DESC LIMIT ? OFFSET ?;")
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := r.db.QueryContext(ctx, query.String(), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]strategy.ApplicationEnvRuntimeBinding, 0)
	for rows.Next() {
		item, err := scanApplicationEnvRuntimeBinding(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *StrategyRepository) UpdateRuntimeBinding(ctx context.Context, id string, input strategy.RuntimeBindingUpdateInput) (strategy.ApplicationEnvRuntimeBinding, error) {
	const q = `
UPDATE application_env_runtime_bindings
SET k8s_cluster_ref_id = ?, namespace = ?, workload_name = ?, updated_at = ?
WHERE id = ?;`
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, q,
		input.K8sClusterRefID,
		input.Namespace,
		input.WorkloadName,
		now.UTC().UnixNano(),
		id,
	)
	if err != nil {
		return strategy.ApplicationEnvRuntimeBinding{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return strategy.ApplicationEnvRuntimeBinding{}, err
	}
	if affected == 0 {
		return strategy.ApplicationEnvRuntimeBinding{}, strategy.ErrRuntimeBindingNotFound
	}
	return r.GetRuntimeBindingByID(ctx, id)
}

func (r *StrategyRepository) DeleteRuntimeBinding(ctx context.Context, id string) error {
	const q = `DELETE FROM application_env_runtime_bindings WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return strategy.ErrRuntimeBindingNotFound
	}
	return nil
}

func (r *StrategyRepository) CreateStrategyBinding(ctx context.Context, item strategy.ApplicationEnvStrategyBinding) error {
	const q = `
INSERT INTO application_env_strategy_bindings (id, application_id, env_code, strategy_template_id, overrides_config, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(ctx, q,
		item.ID,
		item.ApplicationID,
		item.EnvCode,
		item.StrategyTemplateID,
		item.OverridesConfig,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isStrategyBindingDuplicateError(r.dbDriver, err) {
			return strategy.ErrStrategyBindingDuplicated
		}
		return err
	}
	return nil
}

func (r *StrategyRepository) GetStrategyBindingByID(ctx context.Context, id string) (strategy.ApplicationEnvStrategyBinding, error) {
	const q = `
SELECT id, application_id, env_code, strategy_template_id, overrides_config, created_at, updated_at
FROM application_env_strategy_bindings
WHERE id = ?;`
	row := r.db.QueryRowContext(ctx, q, id)
	item, err := scanApplicationEnvStrategyBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return strategy.ApplicationEnvStrategyBinding{}, strategy.ErrStrategyBindingNotFound
		}
		return strategy.ApplicationEnvStrategyBinding{}, err
	}
	return item, nil
}

func (r *StrategyRepository) GetStrategyBindingByAppEnv(ctx context.Context, applicationID, envCode string) (strategy.ApplicationEnvStrategyBinding, error) {
	const q = `
SELECT id, application_id, env_code, strategy_template_id, overrides_config, created_at, updated_at
FROM application_env_strategy_bindings
WHERE application_id = ? AND env_code = ?;`
	row := r.db.QueryRowContext(ctx, q, applicationID, envCode)
	item, err := scanApplicationEnvStrategyBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return strategy.ApplicationEnvStrategyBinding{}, strategy.ErrStrategyBindingNotFound
		}
		return strategy.ApplicationEnvStrategyBinding{}, err
	}
	return item, nil
}

func (r *StrategyRepository) ListStrategyBindings(ctx context.Context, filter strategy.StrategyBindingListFilter) ([]strategy.ApplicationEnvStrategyBinding, int64, error) {
	args := make([]any, 0, 2)
	where := make([]string, 0, 2)
	if filter.ApplicationID != "" {
		where = append(where, "application_id = ?")
		args = append(args, filter.ApplicationID)
	}
	if filter.EnvCode != "" {
		where = append(where, "env_code = ?")
		args = append(args, filter.EnvCode)
	}

	countSQL := strings.Builder{}
	countSQL.WriteString("SELECT COUNT(1) FROM application_env_strategy_bindings")
	if len(where) > 0 {
		countSQL.WriteString(" WHERE ")
		countSQL.WriteString(strings.Join(where, " AND "))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL.String(), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := strings.Builder{}
	query.WriteString(`
SELECT id, application_id, env_code, strategy_template_id, overrides_config, created_at, updated_at
FROM application_env_strategy_bindings`)
	if len(where) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(where, " AND "))
	}
	query.WriteString(" ORDER BY created_at DESC LIMIT ? OFFSET ?;")
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := r.db.QueryContext(ctx, query.String(), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]strategy.ApplicationEnvStrategyBinding, 0)
	for rows.Next() {
		item, err := scanApplicationEnvStrategyBinding(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *StrategyRepository) UpdateStrategyBinding(ctx context.Context, id string, input strategy.StrategyBindingUpdateInput) (strategy.ApplicationEnvStrategyBinding, error) {
	const q = `
UPDATE application_env_strategy_bindings
SET strategy_template_id = ?, overrides_config = ?, updated_at = ?
WHERE id = ?;`
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, q,
		input.StrategyTemplateID,
		input.OverridesConfig,
		now.UTC().UnixNano(),
		id,
	)
	if err != nil {
		return strategy.ApplicationEnvStrategyBinding{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return strategy.ApplicationEnvStrategyBinding{}, err
	}
	if affected == 0 {
		return strategy.ApplicationEnvStrategyBinding{}, strategy.ErrStrategyBindingNotFound
	}
	return r.GetStrategyBindingByID(ctx, id)
}

func (r *StrategyRepository) DeleteStrategyBinding(ctx context.Context, id string) error {
	const q = `DELETE FROM application_env_strategy_bindings WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return strategy.ErrStrategyBindingNotFound
	}
	return nil
}

type strategyTemplateScanner interface{ Scan(dest ...any) error }

func scanReleaseStrategyTemplate(s strategyTemplateScanner) (strategy.ReleaseStrategyTemplate, error) {
	var item strategy.ReleaseStrategyTemplate
	var engineRaw string
	var typeRaw string
	var statusRaw string
	var createdAt int64
	var updatedAt int64
	if err := s.Scan(
		&item.ID,
		&item.Name,
		&engineRaw,
		&typeRaw,
		&item.StrategyConfig,
		&item.Description,
		&statusRaw,
		&createdAt,
		&updatedAt,
	); err != nil {
		return strategy.ReleaseStrategyTemplate{}, err
	}
	item.StrategyEngine = strategy.StrategyEngine(engineRaw)
	item.StrategyType = strategy.StrategyType(typeRaw)
	item.Status = strategy.TemplateStatus(statusRaw)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

type runtimeBindingScanner interface{ Scan(dest ...any) error }

func scanApplicationEnvRuntimeBinding(s runtimeBindingScanner) (strategy.ApplicationEnvRuntimeBinding, error) {
	var item strategy.ApplicationEnvRuntimeBinding
	var createdAt int64
	var updatedAt int64
	if err := s.Scan(
		&item.ID,
		&item.ApplicationID,
		&item.EnvCode,
		&item.K8sClusterRefID,
		&item.Namespace,
		&item.WorkloadName,
		&createdAt,
		&updatedAt,
	); err != nil {
		return strategy.ApplicationEnvRuntimeBinding{}, err
	}
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

type strategyBindingScanner interface{ Scan(dest ...any) error }

func scanApplicationEnvStrategyBinding(s strategyBindingScanner) (strategy.ApplicationEnvStrategyBinding, error) {
	var item strategy.ApplicationEnvStrategyBinding
	var createdAt int64
	var updatedAt int64
	if err := s.Scan(
		&item.ID,
		&item.ApplicationID,
		&item.EnvCode,
		&item.StrategyTemplateID,
		&item.OverridesConfig,
		&createdAt,
		&updatedAt,
	); err != nil {
		return strategy.ApplicationEnvStrategyBinding{}, err
	}
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

func isStrategyDuplicateNameError(dbDriver string, err error) bool {
	switch dbDriver {
	case "mysql":
		var mysqlErr *mysqlDriver.MySQLError
		return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
	case "sqlite":
		return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
	default:
		return false
	}
}

func isStrategyBindingDuplicateError(dbDriver string, err error) bool {
	switch dbDriver {
	case "mysql":
		var mysqlErr *mysqlDriver.MySQLError
		return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
	case "sqlite":
		return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
	default:
		return false
	}
}
