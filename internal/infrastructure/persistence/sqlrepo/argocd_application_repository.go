package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/argocdapp"
)

type ArgoCDApplicationRepository struct {
	db       *sql.DB
	dbDriver string
}

// NewArgoCDApplicationRepository 创建并返回对应组件实例。
func NewArgoCDApplicationRepository(db *sql.DB, dbDriver string) *ArgoCDApplicationRepository {
	return &ArgoCDApplicationRepository{db: db, dbDriver: strings.ToLower(strings.TrimSpace(dbDriver))}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (r *ArgoCDApplicationRepository) InitSchema(ctx context.Context) error {
	var statements []string
	switch r.dbDriver {
	case "mysql":
		statements = []string{
			`CREATE TABLE IF NOT EXISTS argocd_instance (
	id VARCHAR(64) PRIMARY KEY,
	instance_code VARCHAR(100) NOT NULL,
	name VARCHAR(120) NOT NULL,
	base_url VARCHAR(500) NOT NULL,
	insecure_skip_verify TINYINT(1) NOT NULL DEFAULT 0,
	auth_mode VARCHAR(32) NOT NULL DEFAULT '',
	token_ciphertext TEXT NOT NULL,
	username VARCHAR(120) NOT NULL DEFAULT '',
	password_ciphertext TEXT NOT NULL,
	gitops_instance_id VARCHAR(64) NOT NULL DEFAULT '',
	cluster_name VARCHAR(120) NOT NULL DEFAULT '',
	default_namespace VARCHAR(120) NOT NULL DEFAULT '',
	status VARCHAR(20) NOT NULL DEFAULT 'active',
	health_status VARCHAR(32) NOT NULL DEFAULT '',
	last_check_at BIGINT NOT NULL DEFAULT 0,
	remark VARCHAR(500) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_argocd_instance_code (instance_code),
	UNIQUE KEY uk_argocd_instance_base_url (base_url),
	KEY idx_argocd_instance_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS argocd_env_binding (
	id VARCHAR(64) PRIMARY KEY,
	env_code VARCHAR(64) NOT NULL,
	argocd_instance_id VARCHAR(64) NOT NULL,
	priority INT NOT NULL DEFAULT 1,
	status VARCHAR(20) NOT NULL DEFAULT 'active',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_argocd_env_binding_env (env_code),
	KEY idx_argocd_env_binding_instance (argocd_instance_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS argocd_application (
	id VARCHAR(64) PRIMARY KEY,
	argocd_instance_id VARCHAR(64) NOT NULL DEFAULT '',
	instance_code VARCHAR(100) NOT NULL DEFAULT '',
	instance_name VARCHAR(120) NOT NULL DEFAULT '',
	cluster_name VARCHAR(120) NOT NULL DEFAULT '',
	instance_base_url VARCHAR(500) NOT NULL DEFAULT '',
	app_name VARCHAR(200) NOT NULL,
	project VARCHAR(100) NOT NULL DEFAULT '',
	repo_url VARCHAR(500) NOT NULL DEFAULT '',
	source_path VARCHAR(500) NOT NULL DEFAULT '',
	target_revision VARCHAR(200) NOT NULL DEFAULT '',
	dest_server VARCHAR(500) NOT NULL DEFAULT '',
	dest_namespace VARCHAR(200) NOT NULL DEFAULT '',
	sync_status VARCHAR(50) NOT NULL DEFAULT '',
	health_status VARCHAR(50) NOT NULL DEFAULT '',
	operation_phase VARCHAR(50) NOT NULL DEFAULT '',
	argocd_url VARCHAR(500) NOT NULL DEFAULT '',
	status VARCHAR(20) NOT NULL DEFAULT 'active',
	raw_meta JSON NULL,
	last_synced_at BIGINT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_argocd_application_instance_name (argocd_instance_id, app_name),
	KEY idx_argocd_application_instance (argocd_instance_id),
	KEY idx_argocd_project (project),
	KEY idx_argocd_sync_status (sync_status),
	KEY idx_argocd_health_status (health_status),
	KEY idx_argocd_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}
	case "sqlite":
		statements = []string{
			`CREATE TABLE IF NOT EXISTS argocd_instance (
	id TEXT PRIMARY KEY,
	instance_code TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	base_url TEXT NOT NULL UNIQUE,
	insecure_skip_verify INTEGER NOT NULL DEFAULT 0,
	auth_mode TEXT NOT NULL DEFAULT '',
	token_ciphertext TEXT NOT NULL DEFAULT '',
	username TEXT NOT NULL DEFAULT '',
	password_ciphertext TEXT NOT NULL DEFAULT '',
	gitops_instance_id TEXT NOT NULL DEFAULT '',
	cluster_name TEXT NOT NULL DEFAULT '',
	default_namespace TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	health_status TEXT NOT NULL DEFAULT '',
	last_check_at INTEGER NOT NULL DEFAULT 0,
	remark TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_argocd_instance_status ON argocd_instance (status);`,
			`CREATE TABLE IF NOT EXISTS argocd_env_binding (
	id TEXT PRIMARY KEY,
	env_code TEXT NOT NULL UNIQUE,
	argocd_instance_id TEXT NOT NULL,
	priority INTEGER NOT NULL DEFAULT 1,
	status TEXT NOT NULL DEFAULT 'active',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_argocd_env_binding_instance ON argocd_env_binding (argocd_instance_id);`,
			`CREATE TABLE IF NOT EXISTS argocd_application (
	id TEXT PRIMARY KEY,
	argocd_instance_id TEXT NOT NULL DEFAULT '',
	instance_code TEXT NOT NULL DEFAULT '',
	instance_name TEXT NOT NULL DEFAULT '',
	cluster_name TEXT NOT NULL DEFAULT '',
	instance_base_url TEXT NOT NULL DEFAULT '',
	app_name TEXT NOT NULL,
	project TEXT NOT NULL DEFAULT '',
	repo_url TEXT NOT NULL DEFAULT '',
	source_path TEXT NOT NULL DEFAULT '',
	target_revision TEXT NOT NULL DEFAULT '',
	dest_server TEXT NOT NULL DEFAULT '',
	dest_namespace TEXT NOT NULL DEFAULT '',
	sync_status TEXT NOT NULL DEFAULT '',
	health_status TEXT NOT NULL DEFAULT '',
	operation_phase TEXT NOT NULL DEFAULT '',
	argocd_url TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	raw_meta TEXT NOT NULL DEFAULT '',
	last_synced_at INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE(argocd_instance_id, app_name)
);`,
			`CREATE INDEX IF NOT EXISTS idx_argocd_application_instance ON argocd_application (argocd_instance_id);`,
			`CREATE INDEX IF NOT EXISTS idx_argocd_project ON argocd_application (project);`,
			`CREATE INDEX IF NOT EXISTS idx_argocd_sync_status ON argocd_application (sync_status);`,
			`CREATE INDEX IF NOT EXISTS idx_argocd_health_status ON argocd_application (health_status);`,
			`CREATE INDEX IF NOT EXISTS idx_argocd_status ON argocd_application (status);`,
		}
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}

	for _, stmt := range statements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return r.migrateSchema(ctx)
}

// migrateSchema 封装当前模块的业务处理逻辑。
func (r *ArgoCDApplicationRepository) migrateSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
		columnStatements := []struct {
			table  string
			column string
			ddl    string
		}{
			{"argocd_instance", "gitops_instance_id", `ALTER TABLE argocd_instance ADD COLUMN gitops_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER password_ciphertext;`},
			{"argocd_application", "argocd_instance_id", `ALTER TABLE argocd_application ADD COLUMN argocd_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER id;`},
			{"argocd_application", "instance_code", `ALTER TABLE argocd_application ADD COLUMN instance_code VARCHAR(100) NOT NULL DEFAULT '' AFTER argocd_instance_id;`},
			{"argocd_application", "instance_name", `ALTER TABLE argocd_application ADD COLUMN instance_name VARCHAR(120) NOT NULL DEFAULT '' AFTER instance_code;`},
			{"argocd_application", "cluster_name", `ALTER TABLE argocd_application ADD COLUMN cluster_name VARCHAR(120) NOT NULL DEFAULT '' AFTER instance_name;`},
			{"argocd_application", "instance_base_url", `ALTER TABLE argocd_application ADD COLUMN instance_base_url VARCHAR(500) NOT NULL DEFAULT '' AFTER cluster_name;`},
		}
		for _, item := range columnStatements {
			exists, err := r.mysqlColumnExists(ctx, item.table, item.column)
			if err != nil {
				return err
			}
			if !exists {
				if _, err := r.db.ExecContext(ctx, item.ddl); err != nil {
					return err
				}
			}
		}
		oldUniqueExists, err := r.mysqlIndexExists(ctx, "argocd_application", "uk_argocd_application_name")
		if err != nil {
			return err
		}
		if oldUniqueExists {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE argocd_application DROP INDEX uk_argocd_application_name;`); err != nil {
				return err
			}
		}
		newUniqueExists, err := r.mysqlIndexExists(ctx, "argocd_application", "uk_argocd_application_instance_name")
		if err != nil {
			return err
		}
		if !newUniqueExists {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE argocd_application ADD UNIQUE KEY uk_argocd_application_instance_name (argocd_instance_id, app_name);`); err != nil {
				return err
			}
		}
		instanceIdxExists, err := r.mysqlIndexExists(ctx, "argocd_application", "idx_argocd_application_instance")
		if err != nil {
			return err
		}
		if !instanceIdxExists {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE argocd_application ADD KEY idx_argocd_application_instance (argocd_instance_id);`); err != nil {
				return err
			}
		}
	case "sqlite":
		columns, err := r.sqliteTableColumns(ctx, "argocd_instance")
		if err != nil {
			return err
		}
		if _, ok := columns["gitops_instance_id"]; !ok {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE argocd_instance ADD COLUMN gitops_instance_id TEXT NOT NULL DEFAULT '';`); err != nil {
				return err
			}
		}
		columns, err = r.sqliteTableColumns(ctx, "argocd_application")
		if err != nil {
			return err
		}
		if _, ok := columns["argocd_instance_id"]; !ok {
			if err := r.rebuildSQLiteApplicationTable(ctx); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	return nil
}

// CleanupLegacyApplications 封装当前模块的业务处理逻辑。
func (r *ArgoCDApplicationRepository) CleanupLegacyApplications(ctx context.Context) error {
	// 多实例改造前，argocd_application 里允许存在空 argocd_instance_id 的历史快照。
	// 启用多实例后，这类记录会和新的实例化快照重复显示；启动时清掉这些旧数据。
	const q = `
DELETE legacy
FROM argocd_application legacy
INNER JOIN argocd_application actual
	ON legacy.app_name = actual.app_name
WHERE COALESCE(legacy.argocd_instance_id, '') = ''
	AND COALESCE(actual.argocd_instance_id, '') <> '';`

	if r.dbDriver == "sqlite" {
		_, err := r.db.ExecContext(ctx, `
DELETE FROM argocd_application
WHERE COALESCE(argocd_instance_id, '') = ''
	AND app_name IN (
		SELECT DISTINCT app_name
		FROM argocd_application
		WHERE COALESCE(argocd_instance_id, '') <> ''
	);`)
		return err
	}

	_, err := r.db.ExecContext(ctx, q)
	return err
}

// rebuildSQLiteApplicationTable 组装业务执行所需的输入数据。
func (r *ArgoCDApplicationRepository) rebuildSQLiteApplicationTable(ctx context.Context) error {
	statements := []string{
		`ALTER TABLE argocd_application RENAME TO argocd_application_legacy;`,
		`CREATE TABLE argocd_application (
	id TEXT PRIMARY KEY,
	argocd_instance_id TEXT NOT NULL DEFAULT '',
	instance_code TEXT NOT NULL DEFAULT '',
	instance_name TEXT NOT NULL DEFAULT '',
	cluster_name TEXT NOT NULL DEFAULT '',
	instance_base_url TEXT NOT NULL DEFAULT '',
	app_name TEXT NOT NULL,
	project TEXT NOT NULL DEFAULT '',
	repo_url TEXT NOT NULL DEFAULT '',
	source_path TEXT NOT NULL DEFAULT '',
	target_revision TEXT NOT NULL DEFAULT '',
	dest_server TEXT NOT NULL DEFAULT '',
	dest_namespace TEXT NOT NULL DEFAULT '',
	sync_status TEXT NOT NULL DEFAULT '',
	health_status TEXT NOT NULL DEFAULT '',
	operation_phase TEXT NOT NULL DEFAULT '',
	argocd_url TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	raw_meta TEXT NOT NULL DEFAULT '',
	last_synced_at INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE(argocd_instance_id, app_name)
);`,
		`INSERT INTO argocd_application (
	id, argocd_instance_id, instance_code, instance_name, cluster_name, instance_base_url, app_name, project, repo_url, source_path, target_revision,
	dest_server, dest_namespace, sync_status, health_status, operation_phase, argocd_url, status, raw_meta, last_synced_at, created_at, updated_at
)
SELECT id, '', '', '', '', '', app_name, project, repo_url, source_path, target_revision,
	dest_server, dest_namespace, sync_status, health_status, operation_phase, argocd_url, status, raw_meta, last_synced_at, created_at, updated_at
FROM argocd_application_legacy;`,
		`DROP TABLE argocd_application_legacy;`,
		`CREATE INDEX IF NOT EXISTS idx_argocd_application_instance ON argocd_application (argocd_instance_id);`,
		`CREATE INDEX IF NOT EXISTS idx_argocd_project ON argocd_application (project);`,
		`CREATE INDEX IF NOT EXISTS idx_argocd_sync_status ON argocd_application (sync_status);`,
		`CREATE INDEX IF NOT EXISTS idx_argocd_health_status ON argocd_application (health_status);`,
		`CREATE INDEX IF NOT EXISTS idx_argocd_status ON argocd_application (status);`,
	}
	for _, stmt := range statements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// UpsertInstance 封装当前模块的业务处理逻辑。
func (r *ArgoCDApplicationRepository) UpsertInstance(ctx context.Context, item domain.Instance) (domain.Instance, error) {
	now := item.UpdatedAt.UTC().UnixNano()
	createdAt := item.CreatedAt.UTC().UnixNano()
	if createdAt == 0 {
		createdAt = item.UpdatedAt.UTC().UnixNano()
	}
	encryptedToken, err := encryptStoredSecret(strings.TrimSpace(item.Token))
	if err != nil {
		return domain.Instance{}, err
	}
	encryptedPassword, err := encryptStoredSecret(strings.TrimSpace(item.Password))
	if err != nil {
		return domain.Instance{}, err
	}
	var q string
	switch r.dbDriver {
	case "mysql":
		q = `
INSERT INTO argocd_instance (
	id, instance_code, name, base_url, insecure_skip_verify, auth_mode, token_ciphertext, username, password_ciphertext,
	gitops_instance_id, cluster_name, default_namespace, status, health_status, last_check_at, remark, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	name = VALUES(name),
	base_url = VALUES(base_url),
	insecure_skip_verify = VALUES(insecure_skip_verify),
	auth_mode = VALUES(auth_mode),
	token_ciphertext = VALUES(token_ciphertext),
	username = VALUES(username),
	password_ciphertext = VALUES(password_ciphertext),
	gitops_instance_id = VALUES(gitops_instance_id),
	cluster_name = VALUES(cluster_name),
	default_namespace = VALUES(default_namespace),
	status = VALUES(status),
	health_status = VALUES(health_status),
	last_check_at = VALUES(last_check_at),
	remark = VALUES(remark),
	updated_at = VALUES(updated_at);`
	case "sqlite":
		q = `
INSERT INTO argocd_instance (
	id, instance_code, name, base_url, insecure_skip_verify, auth_mode, token_ciphertext, username, password_ciphertext,
	gitops_instance_id, cluster_name, default_namespace, status, health_status, last_check_at, remark, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(instance_code) DO UPDATE SET
	name = excluded.name,
	base_url = excluded.base_url,
	insecure_skip_verify = excluded.insecure_skip_verify,
	auth_mode = excluded.auth_mode,
	token_ciphertext = excluded.token_ciphertext,
	username = excluded.username,
	password_ciphertext = excluded.password_ciphertext,
	gitops_instance_id = excluded.gitops_instance_id,
	cluster_name = excluded.cluster_name,
	default_namespace = excluded.default_namespace,
	status = excluded.status,
	health_status = excluded.health_status,
	last_check_at = excluded.last_check_at,
	remark = excluded.remark,
	updated_at = excluded.updated_at;`
	default:
		return domain.Instance{}, fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	if _, err := r.db.ExecContext(ctx, q,
		item.ID,
		strings.TrimSpace(item.InstanceCode),
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.BaseURL),
		argocdBoolToTinyInt(item.InsecureSkipVerify),
		strings.TrimSpace(item.AuthMode),
		encryptedToken,
		strings.TrimSpace(item.Username),
		encryptedPassword,
		strings.TrimSpace(item.GitOpsInstanceID),
		strings.TrimSpace(item.ClusterName),
		strings.TrimSpace(item.DefaultNamespace),
		string(item.Status),
		strings.TrimSpace(item.HealthStatus),
		instanceLastCheckUnixNano(item.LastCheckAt),
		strings.TrimSpace(item.Remark),
		createdAt,
		now,
	); err != nil {
		return domain.Instance{}, err
	}
	return r.GetInstanceByCode(ctx, item.InstanceCode)
}

// CreateInstance 创建业务资源并返回处理结果。
func (r *ArgoCDApplicationRepository) CreateInstance(ctx context.Context, item domain.Instance) (domain.Instance, error) {
	encryptedToken, err := encryptStoredSecret(strings.TrimSpace(item.Token))
	if err != nil {
		return domain.Instance{}, err
	}
	encryptedPassword, err := encryptStoredSecret(strings.TrimSpace(item.Password))
	if err != nil {
		return domain.Instance{}, err
	}
	const mysqlQ = `
INSERT INTO argocd_instance (
	id, instance_code, name, base_url, insecure_skip_verify, auth_mode, token_ciphertext, username, password_ciphertext,
	gitops_instance_id, cluster_name, default_namespace, status, health_status, last_check_at, remark, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	const sqliteQ = mysqlQ
	q := mysqlQ
	if r.dbDriver == "sqlite" {
		q = sqliteQ
	}
	if _, err := r.db.ExecContext(ctx, q,
		item.ID,
		strings.TrimSpace(item.InstanceCode),
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.BaseURL),
		argocdBoolToTinyInt(item.InsecureSkipVerify),
		strings.TrimSpace(item.AuthMode),
		encryptedToken,
		strings.TrimSpace(item.Username),
		encryptedPassword,
		strings.TrimSpace(item.GitOpsInstanceID),
		strings.TrimSpace(item.ClusterName),
		strings.TrimSpace(item.DefaultNamespace),
		string(item.Status),
		strings.TrimSpace(item.HealthStatus),
		instanceLastCheckUnixNano(item.LastCheckAt),
		strings.TrimSpace(item.Remark),
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	); err != nil {
		return domain.Instance{}, err
	}
	return r.GetInstanceByID(ctx, item.ID)
}

// UpdateInstance 更新业务资源并返回处理结果。
func (r *ArgoCDApplicationRepository) UpdateInstance(ctx context.Context, item domain.Instance) (domain.Instance, error) {
	encryptedToken, err := encryptStoredSecret(strings.TrimSpace(item.Token))
	if err != nil {
		return domain.Instance{}, err
	}
	encryptedPassword, err := encryptStoredSecret(strings.TrimSpace(item.Password))
	if err != nil {
		return domain.Instance{}, err
	}
	const q = `
UPDATE argocd_instance
SET instance_code = ?, name = ?, base_url = ?, insecure_skip_verify = ?, auth_mode = ?, token_ciphertext = ?, username = ?, password_ciphertext = ?,
	gitops_instance_id = ?, cluster_name = ?, default_namespace = ?, status = ?, health_status = ?, last_check_at = ?, remark = ?, updated_at = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q,
		strings.TrimSpace(item.InstanceCode),
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.BaseURL),
		argocdBoolToTinyInt(item.InsecureSkipVerify),
		strings.TrimSpace(item.AuthMode),
		encryptedToken,
		strings.TrimSpace(item.Username),
		encryptedPassword,
		strings.TrimSpace(item.GitOpsInstanceID),
		strings.TrimSpace(item.ClusterName),
		strings.TrimSpace(item.DefaultNamespace),
		string(item.Status),
		strings.TrimSpace(item.HealthStatus),
		instanceLastCheckUnixNano(item.LastCheckAt),
		strings.TrimSpace(item.Remark),
		item.UpdatedAt.UTC().UnixNano(),
		strings.TrimSpace(item.ID),
	)
	if err != nil {
		return domain.Instance{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Instance{}, err
	}
	if affected == 0 {
		return domain.Instance{}, domain.ErrInstanceNotFound
	}
	return r.GetInstanceByID(ctx, item.ID)
}

// GetInstanceByID 查询并返回指定资源数据。
func (r *ArgoCDApplicationRepository) GetInstanceByID(ctx context.Context, id string) (domain.Instance, error) {
	const q = `
SELECT i.id, i.instance_code, i.name, i.base_url, i.insecure_skip_verify, i.auth_mode, i.token_ciphertext, i.username, i.password_ciphertext,
	i.gitops_instance_id, COALESCE(g.instance_code, ''), COALESCE(g.name, ''), i.cluster_name, i.default_namespace, i.status, i.health_status, i.last_check_at, i.remark, i.created_at, i.updated_at
FROM argocd_instance i
LEFT JOIN gitops_instance g ON g.id = i.gitops_instance_id
WHERE i.id = ?;`
	item, err := scanArgoCDInstance(r.db.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Instance{}, domain.ErrInstanceNotFound
		}
		return domain.Instance{}, err
	}
	return item, nil
}

// GetInstanceByCode 查询并返回指定资源数据。
func (r *ArgoCDApplicationRepository) GetInstanceByCode(ctx context.Context, code string) (domain.Instance, error) {
	const q = `
SELECT i.id, i.instance_code, i.name, i.base_url, i.insecure_skip_verify, i.auth_mode, i.token_ciphertext, i.username, i.password_ciphertext,
	i.gitops_instance_id, COALESCE(g.instance_code, ''), COALESCE(g.name, ''), i.cluster_name, i.default_namespace, i.status, i.health_status, i.last_check_at, i.remark, i.created_at, i.updated_at
FROM argocd_instance i
LEFT JOIN gitops_instance g ON g.id = i.gitops_instance_id
WHERE i.instance_code = ?;`
	item, err := scanArgoCDInstance(r.db.QueryRowContext(ctx, q, strings.TrimSpace(code)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Instance{}, domain.ErrInstanceNotFound
		}
		return domain.Instance{}, err
	}
	return item, nil
}

// ListInstances 查询并返回列表数据。
func (r *ArgoCDApplicationRepository) ListInstances(ctx context.Context, filter domain.InstanceListFilter) ([]domain.Instance, int64, error) {
	args := make([]any, 0, 8)
	where := make([]string, 0, 2)
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		where = append(where, "(i.instance_code LIKE ? OR i.name LIKE ? OR i.cluster_name LIKE ? OR i.base_url LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like, like, like)
	}
	if filter.Status != "" {
		where = append(where, "i.status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	countSQL := `SELECT COUNT(1) FROM argocd_instance i`
	if len(where) > 0 {
		countSQL += " WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	querySQL := `SELECT i.id, i.instance_code, i.name, i.base_url, i.insecure_skip_verify, i.auth_mode, i.token_ciphertext, i.username, i.password_ciphertext,
	i.gitops_instance_id, COALESCE(g.instance_code, ''), COALESCE(g.name, ''), i.cluster_name, i.default_namespace, i.status, i.health_status, i.last_check_at, i.remark, i.created_at, i.updated_at
FROM argocd_instance i
LEFT JOIN gitops_instance g ON g.id = i.gitops_instance_id`
	if len(where) > 0 {
		querySQL += " WHERE " + strings.Join(where, " AND ")
	}
	querySQL += " ORDER BY i.created_at DESC LIMIT ? OFFSET ?"
	rows, err := r.db.QueryContext(ctx, querySQL, append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.Instance, 0)
	for rows.Next() {
		item, scanErr := scanArgoCDInstance(rows)
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

// ListActiveInstances 查询并返回列表数据。
func (r *ArgoCDApplicationRepository) ListActiveInstances(ctx context.Context) ([]domain.Instance, error) {
	const q = `
SELECT i.id, i.instance_code, i.name, i.base_url, i.insecure_skip_verify, i.auth_mode, i.token_ciphertext, i.username, i.password_ciphertext,
	i.gitops_instance_id, COALESCE(g.instance_code, ''), COALESCE(g.name, ''), i.cluster_name, i.default_namespace, i.status, i.health_status, i.last_check_at, i.remark, i.created_at, i.updated_at
FROM argocd_instance i
LEFT JOIN gitops_instance g ON g.id = i.gitops_instance_id
WHERE i.status = ? ORDER BY i.instance_code ASC;`
	rows, err := r.db.QueryContext(ctx, q, string(domain.StatusActive))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Instance, 0)
	for rows.Next() {
		item, scanErr := scanArgoCDInstance(rows)
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

// UpdateInstanceHealth 更新业务资源并返回处理结果。
func (r *ArgoCDApplicationRepository) UpdateInstanceHealth(ctx context.Context, id string, healthStatus string, checkedAt time.Time) error {
	const q = `UPDATE argocd_instance SET health_status = ?, last_check_at = ?, updated_at = ? WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, strings.TrimSpace(healthStatus), instanceLastCheckUnixNano(checkedAt), checkedAt.UTC().UnixNano(), strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrInstanceNotFound
	}
	return nil
}

// ListEnvBindings 查询并返回列表数据。
func (r *ArgoCDApplicationRepository) ListEnvBindings(ctx context.Context) ([]domain.EnvBinding, error) {
	const q = `
SELECT b.id, b.env_code, b.argocd_instance_id, i.instance_code, i.name, i.cluster_name, b.priority, b.status, b.created_at, b.updated_at
FROM argocd_env_binding b
LEFT JOIN argocd_instance i ON i.id = b.argocd_instance_id
ORDER BY b.env_code ASC, b.priority ASC, b.created_at ASC;`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.EnvBinding, 0)
	for rows.Next() {
		item, scanErr := scanArgoCDEnvBinding(rows)
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

// ReplaceEnvBindings 封装当前模块的业务处理逻辑。
func (r *ArgoCDApplicationRepository) ReplaceEnvBindings(ctx context.Context, items []domain.EnvBinding) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `DELETE FROM argocd_env_binding`); err != nil {
		return err
	}
	if len(items) == 0 {
		err = tx.Commit()
		return err
	}
	const q = `
INSERT INTO argocd_env_binding (id, env_code, argocd_instance_id, priority, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);`
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range items {
		if _, err = stmt.ExecContext(ctx,
			item.ID,
			strings.TrimSpace(item.EnvCode),
			strings.TrimSpace(item.ArgoCDInstanceID),
			item.Priority,
			string(item.Status),
			item.CreatedAt.UTC().UnixNano(),
			item.UpdatedAt.UTC().UnixNano(),
		); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

// ResolveInstanceByEnv 解析上下文数据，得到后续流程需要的结果。
func (r *ArgoCDApplicationRepository) ResolveInstanceByEnv(ctx context.Context, envCode string) (domain.Instance, error) {
	envCode = strings.TrimSpace(envCode)
	if envCode == "" {
		return domain.Instance{}, domain.ErrEnvBindingNotFound
	}
	const q = `
SELECT i.id, i.instance_code, i.name, i.base_url, i.insecure_skip_verify, i.auth_mode, i.token_ciphertext, i.username, i.password_ciphertext,
	i.gitops_instance_id, COALESCE(g.instance_code, ''), COALESCE(g.name, ''), i.cluster_name, i.default_namespace, i.status, i.health_status, i.last_check_at, i.remark, i.created_at, i.updated_at
FROM argocd_env_binding b
INNER JOIN argocd_instance i ON i.id = b.argocd_instance_id
LEFT JOIN gitops_instance g ON g.id = i.gitops_instance_id
WHERE b.env_code = ? AND b.status = ? AND i.status = ?
ORDER BY b.priority ASC, b.created_at ASC
LIMIT 1;`
	item, err := scanArgoCDInstance(r.db.QueryRowContext(ctx, q, envCode, string(domain.StatusActive), string(domain.StatusActive)))
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.Instance{}, err
	}
	instances, listErr := r.ListActiveInstances(ctx)
	if listErr != nil {
		return domain.Instance{}, listErr
	}
	if len(instances) == 1 {
		return instances[0], nil
	}
	return domain.Instance{}, domain.ErrEnvBindingNotFound
}

// UpsertApplications 封装当前模块的业务处理逻辑。
func (r *ArgoCDApplicationRepository) UpsertApplications(ctx context.Context, items []domain.Application) (created int, updated int, err error) {
	if len(items) == 0 {
		return 0, 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	const countQ = `SELECT COUNT(1) FROM argocd_application WHERE argocd_instance_id = ? AND app_name = ?;`
	const mysqlUpsert = `
INSERT INTO argocd_application (
	id, argocd_instance_id, instance_code, instance_name, cluster_name, instance_base_url, app_name, project, repo_url, source_path, target_revision,
	dest_server, dest_namespace, sync_status, health_status, operation_phase, argocd_url, status, raw_meta, last_synced_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	instance_code = VALUES(instance_code),
	instance_name = VALUES(instance_name),
	cluster_name = VALUES(cluster_name),
	instance_base_url = VALUES(instance_base_url),
	project = VALUES(project),
	repo_url = VALUES(repo_url),
	source_path = VALUES(source_path),
	target_revision = VALUES(target_revision),
	dest_server = VALUES(dest_server),
	dest_namespace = VALUES(dest_namespace),
	sync_status = VALUES(sync_status),
	health_status = VALUES(health_status),
	operation_phase = VALUES(operation_phase),
	argocd_url = VALUES(argocd_url),
	status = VALUES(status),
	raw_meta = VALUES(raw_meta),
	last_synced_at = VALUES(last_synced_at),
	updated_at = VALUES(updated_at);`
	const sqliteUpsert = `
INSERT INTO argocd_application (
	id, argocd_instance_id, instance_code, instance_name, cluster_name, instance_base_url, app_name, project, repo_url, source_path, target_revision,
	dest_server, dest_namespace, sync_status, health_status, operation_phase, argocd_url, status, raw_meta, last_synced_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(argocd_instance_id, app_name) DO UPDATE SET
	instance_code = excluded.instance_code,
	instance_name = excluded.instance_name,
	cluster_name = excluded.cluster_name,
	instance_base_url = excluded.instance_base_url,
	project = excluded.project,
	repo_url = excluded.repo_url,
	source_path = excluded.source_path,
	target_revision = excluded.target_revision,
	dest_server = excluded.dest_server,
	dest_namespace = excluded.dest_namespace,
	sync_status = excluded.sync_status,
	health_status = excluded.health_status,
	operation_phase = excluded.operation_phase,
	argocd_url = excluded.argocd_url,
	status = excluded.status,
	raw_meta = excluded.raw_meta,
	last_synced_at = excluded.last_synced_at,
	updated_at = excluded.updated_at;`
	stmtText := mysqlUpsert
	if r.dbDriver == "sqlite" {
		stmtText = sqliteUpsert
	}
	stmt, err := tx.PrepareContext(ctx, stmtText)
	if err != nil {
		return 0, 0, err
	}
	defer stmt.Close()
	for _, item := range items {
		var exists int
		if err = tx.QueryRowContext(ctx, countQ, strings.TrimSpace(item.ArgoCDInstanceID), strings.TrimSpace(item.AppName)).Scan(&exists); err != nil {
			return 0, 0, err
		}
		if exists > 0 {
			updated++
		} else {
			created++
		}
		if _, err = stmt.ExecContext(ctx,
			item.ID,
			strings.TrimSpace(item.ArgoCDInstanceID),
			strings.TrimSpace(item.InstanceCode),
			strings.TrimSpace(item.InstanceName),
			strings.TrimSpace(item.ClusterName),
			strings.TrimSpace(item.InstanceBaseURL),
			strings.TrimSpace(item.AppName),
			strings.TrimSpace(item.Project),
			strings.TrimSpace(item.RepoURL),
			strings.TrimSpace(item.SourcePath),
			strings.TrimSpace(item.TargetRevision),
			strings.TrimSpace(item.DestServer),
			strings.TrimSpace(item.DestNamespace),
			strings.TrimSpace(item.SyncStatus),
			strings.TrimSpace(item.HealthStatus),
			strings.TrimSpace(item.OperationPhase),
			strings.TrimSpace(item.ArgoCDURL),
			string(item.Status),
			strings.TrimSpace(item.RawMeta),
			item.LastSyncedAt.UTC().UnixNano(),
			item.CreatedAt.UTC().UnixNano(),
			item.UpdatedAt.UTC().UnixNano(),
		); err != nil {
			return 0, 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return created, updated, nil
}

// MarkMissingApplicationsInactive 封装当前模块的业务处理逻辑。
func (r *ArgoCDApplicationRepository) MarkMissingApplicationsInactive(ctx context.Context, argocdInstanceID string, keepNames []string, updatedAt time.Time) (int, error) {
	args := make([]any, 0, 3+len(keepNames))
	builder := strings.Builder{}
	builder.WriteString(`UPDATE argocd_application SET status = ?, updated_at = ? WHERE argocd_instance_id = ? AND status <> ?`)
	args = append(args, string(domain.StatusInactive), updatedAt.UTC().UnixNano(), strings.TrimSpace(argocdInstanceID), string(domain.StatusInactive))
	if len(keepNames) > 0 {
		placeholders := make([]string, 0, len(keepNames))
		for _, item := range keepNames {
			placeholders = append(placeholders, "?")
			args = append(args, item)
		}
		builder.WriteString(" AND app_name NOT IN (")
		builder.WriteString(strings.Join(placeholders, ", "))
		builder.WriteString(")")
	}
	result, err := r.db.ExecContext(ctx, builder.String(), args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// ListApplications 查询并返回列表数据。
func (r *ArgoCDApplicationRepository) ListApplications(ctx context.Context, filter domain.ListFilter) ([]domain.Application, int64, error) {
	args := make([]any, 0, 10)
	where := make([]string, 0, 7)
	// 多实例模型启用后，旧版本遗留的空实例记录属于脏数据，不再对外展示。
	where = append(where, "argocd_instance_id <> ''")
	if instanceID := strings.TrimSpace(filter.ArgoCDInstanceID); instanceID != "" {
		where = append(where, "argocd_instance_id = ?")
		args = append(args, instanceID)
	}
	if name := strings.TrimSpace(filter.AppName); name != "" {
		where = append(where, "app_name LIKE ?")
		args = append(args, "%"+name+"%")
	}
	if project := strings.TrimSpace(filter.Project); project != "" {
		where = append(where, "project = ?")
		args = append(args, project)
	}
	if syncStatus := strings.TrimSpace(filter.SyncStatus); syncStatus != "" {
		where = append(where, "sync_status = ?")
		args = append(args, syncStatus)
	}
	if healthStatus := strings.TrimSpace(filter.HealthStatus); healthStatus != "" {
		where = append(where, "health_status = ?")
		args = append(args, healthStatus)
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	countSQL := "SELECT COUNT(1) FROM argocd_application"
	if len(where) > 0 {
		countSQL += " WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	querySQL := `SELECT id, argocd_instance_id, instance_code, instance_name, cluster_name, instance_base_url, app_name, project, repo_url, source_path, target_revision,
	dest_server, dest_namespace, sync_status, health_status, operation_phase, argocd_url, status, raw_meta, last_synced_at, created_at, updated_at
FROM argocd_application`
	if len(where) > 0 {
		querySQL += " WHERE " + strings.Join(where, " AND ")
	}
	querySQL += " ORDER BY instance_code ASC, app_name ASC LIMIT ? OFFSET ?"
	rows, err := r.db.QueryContext(ctx, querySQL, append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.Application, 0)
	for rows.Next() {
		item, scanErr := scanArgoCDApplication(rows)
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

// GetApplicationByID 查询并返回指定资源数据。
func (r *ArgoCDApplicationRepository) GetApplicationByID(ctx context.Context, id string) (domain.Application, error) {
	const q = `SELECT id, argocd_instance_id, instance_code, instance_name, cluster_name, instance_base_url, app_name, project, repo_url, source_path, target_revision,
	dest_server, dest_namespace, sync_status, health_status, operation_phase, argocd_url, status, raw_meta, last_synced_at, created_at, updated_at
FROM argocd_application WHERE id = ?;`
	item, err := scanArgoCDApplication(r.db.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Application{}, domain.ErrNotFound
		}
		return domain.Application{}, err
	}
	return item, nil
}

type argocdInstanceScanner interface{ Scan(dest ...any) error }

type argocdEnvBindingScanner interface{ Scan(dest ...any) error }

type argocdApplicationScanner interface{ Scan(dest ...any) error }

// scanArgoCDInstance 封装当前模块的业务处理逻辑。
func scanArgoCDInstance(scanner argocdInstanceScanner) (domain.Instance, error) {
	var (
		item               domain.Instance
		encryptedToken     string
		encryptedPassword  string
		insecureSkipVerify int
		status             string
		lastCheckAt        int64
		createdAt          int64
		updatedAt          int64
	)
	if err := scanner.Scan(
		&item.ID,
		&item.InstanceCode,
		&item.Name,
		&item.BaseURL,
		&insecureSkipVerify,
		&item.AuthMode,
		&encryptedToken,
		&item.Username,
		&encryptedPassword,
		&item.GitOpsInstanceID,
		&item.GitOpsInstanceCode,
		&item.GitOpsInstanceName,
		&item.ClusterName,
		&item.DefaultNamespace,
		&status,
		&item.HealthStatus,
		&lastCheckAt,
		&item.Remark,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Instance{}, err
	}
	token, err := decryptStoredSecret(encryptedToken)
	if err != nil {
		return domain.Instance{}, err
	}
	password, err := decryptStoredSecret(encryptedPassword)
	if err != nil {
		return domain.Instance{}, err
	}
	item.Token = token
	item.Password = password
	item.InsecureSkipVerify = insecureSkipVerify == 1
	item.Status = domain.Status(status)
	item.LastCheckAt = unixNanoToTime(lastCheckAt)
	item.CreatedAt = unixNanoToTime(createdAt)
	item.UpdatedAt = unixNanoToTime(updatedAt)
	return item, nil
}

// scanArgoCDEnvBinding 封装当前模块的业务处理逻辑。
func scanArgoCDEnvBinding(scanner argocdEnvBindingScanner) (domain.EnvBinding, error) {
	var (
		item      domain.EnvBinding
		status    string
		createdAt int64
		updatedAt int64
	)
	if err := scanner.Scan(
		&item.ID,
		&item.EnvCode,
		&item.ArgoCDInstanceID,
		&item.ArgoCDInstanceCode,
		&item.ArgoCDInstanceName,
		&item.ClusterName,
		&item.Priority,
		&status,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.EnvBinding{}, err
	}
	item.Status = domain.Status(status)
	item.CreatedAt = unixNanoToTime(createdAt)
	item.UpdatedAt = unixNanoToTime(updatedAt)
	return item, nil
}

// scanArgoCDApplication 封装当前模块的业务处理逻辑。
func scanArgoCDApplication(scanner argocdApplicationScanner) (domain.Application, error) {
	var (
		item         domain.Application
		status       string
		rawMeta      sql.NullString
		lastSyncedAt int64
		createdAt    int64
		updatedAt    int64
	)
	if err := scanner.Scan(
		&item.ID,
		&item.ArgoCDInstanceID,
		&item.InstanceCode,
		&item.InstanceName,
		&item.ClusterName,
		&item.InstanceBaseURL,
		&item.AppName,
		&item.Project,
		&item.RepoURL,
		&item.SourcePath,
		&item.TargetRevision,
		&item.DestServer,
		&item.DestNamespace,
		&item.SyncStatus,
		&item.HealthStatus,
		&item.OperationPhase,
		&item.ArgoCDURL,
		&status,
		&rawMeta,
		&lastSyncedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Application{}, err
	}
	item.Status = domain.Status(status)
	item.RawMeta = rawMeta.String
	item.LastSyncedAt = unixNanoToTime(lastSyncedAt)
	item.CreatedAt = unixNanoToTime(createdAt)
	item.UpdatedAt = unixNanoToTime(updatedAt)
	return item, nil
}

// instanceLastCheckUnixNano 检查业务状态并返回校验结果。
func instanceLastCheckUnixNano(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixNano()
}

// unixNanoToTime 封装当前模块的业务处理逻辑。
func unixNanoToTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(0, value).UTC()
}

// argocdBoolToTinyInt 封装当前模块的业务处理逻辑。
func argocdBoolToTinyInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// mysqlColumnExists 封装当前模块的业务处理逻辑。
func (r *ArgoCDApplicationRepository) mysqlColumnExists(ctx context.Context, table, column string) (bool, error) {
	const q = `SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?;`
	var count int
	if err := r.db.QueryRowContext(ctx, q, table, column).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// mysqlIndexExists 封装当前模块的业务处理逻辑。
func (r *ArgoCDApplicationRepository) mysqlIndexExists(ctx context.Context, table, index string) (bool, error) {
	const q = `SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?;`
	var count int
	if err := r.db.QueryRowContext(ctx, q, table, index).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// sqliteTableColumns 封装当前模块的业务处理逻辑。
func (r *ArgoCDApplicationRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s);`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			cid        int
			name       string
			colType    string
			notNull    int
			defaultV   sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultV, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}
