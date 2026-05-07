package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gos/internal/domain/k8sinstance"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type K8sClusterRefRepository struct {
	db       *sql.DB
	dbDriver string
}

func NewK8sClusterRefRepository(db *sql.DB, dbDriver string) *K8sClusterRefRepository {
	return &K8sClusterRefRepository{db: db, dbDriver: strings.ToLower(strings.TrimSpace(dbDriver))}
}

func (r *K8sClusterRefRepository) InitSchema(ctx context.Context) error {
	statements, err := k8sClusterRefSchemaStatements(r.dbDriver)
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

func k8sClusterRefSchemaStatements(dbDriver string) ([]string, error) {
	switch dbDriver {
	case "mysql":
		return []string{
			`CREATE TABLE IF NOT EXISTS k8s_cluster_refs (
	id VARCHAR(64) PRIMARY KEY,
	code VARCHAR(64) NOT NULL,
	cluster_name VARCHAR(128) NOT NULL,
	environment_code VARCHAR(64) NOT NULL,
	api_server VARCHAR(256) NOT NULL,
	default_namespace VARCHAR(128) NOT NULL DEFAULT 'default',
	access_mode VARCHAR(32) NOT NULL DEFAULT 'argocd',
	argocd_instance_id VARCHAR(64) NOT NULL DEFAULT '',
	supports_native_strategy TINYINT(1) NOT NULL DEFAULT 0,
	supports_rollouts TINYINT(1) NOT NULL DEFAULT 0,
	traffic_provider VARCHAR(64) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_k8s_cluster_ref_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}, nil
	case "sqlite":
		return []string{
			`CREATE TABLE IF NOT EXISTS k8s_cluster_refs (
	id TEXT PRIMARY KEY,
	code TEXT NOT NULL UNIQUE,
	cluster_name TEXT NOT NULL,
	environment_code TEXT NOT NULL,
	api_server TEXT NOT NULL,
	default_namespace TEXT NOT NULL DEFAULT 'default',
	access_mode TEXT NOT NULL DEFAULT 'argocd',
	argocd_instance_id TEXT NOT NULL DEFAULT '',
	supports_native_strategy INTEGER NOT NULL DEFAULT 0,
	supports_rollouts INTEGER NOT NULL DEFAULT 0,
	traffic_provider TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported db driver: %s", dbDriver)
	}
}

func (r *K8sClusterRefRepository) migrateSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
	case "sqlite":
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	return nil
}

func (r *K8sClusterRefRepository) Create(ctx context.Context, item k8sinstance.K8sClusterRef) error {
	const q = `
INSERT INTO k8s_cluster_refs (id, code, cluster_name, environment_code, api_server, default_namespace, access_mode, argocd_instance_id, supports_native_strategy, supports_rollouts, traffic_provider, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(ctx, q,
		item.ID,
		item.Code,
		item.ClusterName,
		item.EnvironmentCode,
		item.APIServer,
		item.DefaultNamespace,
		string(item.AccessMode),
		item.ArgoCDInstanceID,
		boolToInt(item.SupportsNativeStrategy),
		boolToInt(item.SupportsRollouts),
		item.TrafficProvider,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isK8sClusterRefDuplicateCodeError(r.dbDriver, err) {
			return k8sinstance.ErrCodeDuplicated
		}
		return err
	}
	return nil
}

func (r *K8sClusterRefRepository) GetByID(ctx context.Context, id string) (k8sinstance.K8sClusterRef, error) {
	const q = `
SELECT id, code, cluster_name, environment_code, api_server, default_namespace, access_mode, argocd_instance_id, supports_native_strategy, supports_rollouts, traffic_provider, created_at, updated_at
FROM k8s_cluster_refs
WHERE id = ?;`
	row := r.db.QueryRowContext(ctx, q, id)
	item, err := scanK8sClusterRef(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return k8sinstance.K8sClusterRef{}, k8sinstance.ErrNotFound
		}
		return k8sinstance.K8sClusterRef{}, err
	}
	return item, nil
}

func (r *K8sClusterRefRepository) GetByCode(ctx context.Context, code string) (k8sinstance.K8sClusterRef, error) {
	const q = `
SELECT id, code, cluster_name, environment_code, api_server, default_namespace, access_mode, argocd_instance_id, supports_native_strategy, supports_rollouts, traffic_provider, created_at, updated_at
FROM k8s_cluster_refs
WHERE code = ?;`
	row := r.db.QueryRowContext(ctx, q, code)
	item, err := scanK8sClusterRef(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return k8sinstance.K8sClusterRef{}, k8sinstance.ErrNotFound
		}
		return k8sinstance.K8sClusterRef{}, err
	}
	return item, nil
}

func (r *K8sClusterRefRepository) List(ctx context.Context, filter k8sinstance.ListFilter) ([]k8sinstance.K8sClusterRef, int64, error) {
	args := make([]any, 0, 3)
	where := make([]string, 0, 2)
	if filter.EnvironmentCode != "" {
		where = append(where, "environment_code = ?")
		args = append(args, filter.EnvironmentCode)
	}
	if filter.Code != "" {
		where = append(where, "code = ?")
		args = append(args, filter.Code)
	}

	countSQL := strings.Builder{}
	countSQL.WriteString("SELECT COUNT(1) FROM k8s_cluster_refs")
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
SELECT id, code, cluster_name, environment_code, api_server, default_namespace, access_mode, argocd_instance_id, supports_native_strategy, supports_rollouts, traffic_provider, created_at, updated_at
FROM k8s_cluster_refs`)
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
	items := make([]k8sinstance.K8sClusterRef, 0)
	for rows.Next() {
		item, err := scanK8sClusterRef(rows)
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

func (r *K8sClusterRefRepository) Update(ctx context.Context, id string, input k8sinstance.UpdateInput) (k8sinstance.K8sClusterRef, error) {
	const q = `
UPDATE k8s_cluster_refs
SET code = ?, cluster_name = ?, environment_code = ?, api_server = ?, default_namespace = ?, argocd_instance_id = ?, supports_native_strategy = ?, supports_rollouts = ?, traffic_provider = ?, updated_at = ?
WHERE id = ?;`

	now := time.Now().UTC()
	supportsNative := boolToInt(false)
	if input.SupportsNativeStrategy != nil {
		supportsNative = boolToInt(*input.SupportsNativeStrategy)
	}
	supportsRollouts := boolToInt(false)
	if input.SupportsRollouts != nil {
		supportsRollouts = boolToInt(*input.SupportsRollouts)
	}

	res, err := r.db.ExecContext(ctx, q,
		input.Code,
		input.ClusterName,
		input.EnvironmentCode,
		input.APIServer,
		input.DefaultNamespace,
		input.ArgoCDInstanceID,
		supportsNative,
		supportsRollouts,
		input.TrafficProvider,
		now.UTC().UnixNano(),
		id,
	)
	if err != nil {
		if isK8sClusterRefDuplicateCodeError(r.dbDriver, err) {
			return k8sinstance.K8sClusterRef{}, k8sinstance.ErrCodeDuplicated
		}
		return k8sinstance.K8sClusterRef{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return k8sinstance.K8sClusterRef{}, err
	}
	if affected == 0 {
		return k8sinstance.K8sClusterRef{}, k8sinstance.ErrNotFound
	}
	return r.GetByID(ctx, id)
}

func (r *K8sClusterRefRepository) Delete(ctx context.Context, id string) error {
	const countQ = `SELECT COUNT(1) FROM application_env_runtime_bindings WHERE k8s_cluster_ref_id = ?;`
	var refs int64
	if err := r.db.QueryRowContext(ctx, countQ, id).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return k8sinstance.ErrInUse
	}

	const q = `DELETE FROM k8s_cluster_refs WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return k8sinstance.ErrNotFound
	}
	return nil
}

type k8sClusterRefScanner interface{ Scan(dest ...any) error }

func scanK8sClusterRef(s k8sClusterRefScanner) (k8sinstance.K8sClusterRef, error) {
	var item k8sinstance.K8sClusterRef
	var accessModeRaw string
	var createdAt int64
	var updatedAt int64
	var supportsNative int
	var supportsRollouts int
	if err := s.Scan(
		&item.ID,
		&item.Code,
		&item.ClusterName,
		&item.EnvironmentCode,
		&item.APIServer,
		&item.DefaultNamespace,
		&accessModeRaw,
		&item.ArgoCDInstanceID,
		&supportsNative,
		&supportsRollouts,
		&item.TrafficProvider,
		&createdAt,
		&updatedAt,
	); err != nil {
		return k8sinstance.K8sClusterRef{}, err
	}
	item.AccessMode = k8sinstance.AccessMode(accessModeRaw)
	item.SupportsNativeStrategy = supportsNative != 0
	item.SupportsRollouts = supportsRollouts != 0
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

func isK8sClusterRefDuplicateCodeError(dbDriver string, err error) bool {
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
